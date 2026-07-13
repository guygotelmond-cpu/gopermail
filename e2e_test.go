package e2e_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"gomail.com/db"
	"gomail.com/proto/control"
	pb "gomail.com/proto/control"
	"gomail.com/smtp"

	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	pgConnStr = "postgres://smtp_user:smtp_password@127.0.0.1:5432/smtp_metadata?sslmode=disable"
	mongoURI  = "mongodb://127.0.0.1:27017"
)

// Helper to start the gRPC Control Server on an open port
func startTestGrpcServer(t *testing.T, rStore *db.PostgresService) (string, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind gRPC listener: %v", err)
	}
	addr := listener.Addr().String()

	baseServer := grpc.NewServer()
	ruleServer := &control.RuleGrpcServer{RStore: rStore}
	pb.RegisterRuleServiceServer(baseServer, ruleServer)

	go func() {
		_ = baseServer.Serve(listener)
	}()

	return addr, func() {
		baseServer.GracefulStop()
		_ = listener.Close()
	}
}

// Helper to start the SMTP Ingestion Server on an open port
func startTestSMTPServer(t *testing.T, rStore *db.PostgresService, dStore *db.MongoService) (string, func()) {
	backend := &smtp.Backend{RStore: rStore, DStore: dStore}
	config := smtp.Config{
		Addr:            "127.0.0.1:0",
		Domain:          "localhost",
		MaxMessageBytes: 10 * 1024 * 1024,
		MaxRecipients:   50,
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
		EnableTLS:       false,
		AllowInsecure:   true,
	}

	server, err := smtp.NewServer(backend, config)
	if err != nil {
		t.Fatalf("Failed to initialize test SMTP server: %v", err)
	}

	listener, err := net.Listen("tcp", config.Addr)
	if err != nil {
		t.Fatalf("Failed to bind tcp listener: %v", err)
	}
	addr := listener.Addr().String()

	go func() {
		_ = server.Serve(listener)
	}()

	return addr, func() {
		_ = server.Close()
	}
}

// Helper to simulate a real SMTP client connection interaction
func sendMockEmail(t *testing.T, addr, from, to, subject, body string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	commands := []string{
		"EHLO localhost\r\n",
		fmt.Sprintf("MAIL FROM:<%s>\r\n", from),
		fmt.Sprintf("RCPT TO:<%s>\r\n", to),
		"DATA\r\n",
		fmt.Sprintf("Subject: %s\r\n\r\n%s\r\n.\r\n", subject, body),
		"QUIT\r\n",
	}

	buf := make([]byte, 1024)
	if _, err := conn.Read(buf); err != nil {
		return err
	}

	for _, cmd := range commands {
		if _, err := conn.Write([]byte(cmd)); err != nil {
			return err
		}
		if _, err := conn.Read(buf); err != nil {
			return err
		}
	}

	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _ = conn.Read(buf) // Graceful goodbye ack lock

	return nil
}

func TestEndToEndPipelineWithGrpc(t *testing.T) {
	rStore, err := db.NewPostgresService(pgConnStr)
	if err != nil {
		t.Fatalf("Postgres connection failure: %v", err)
	}
	dStore, err := db.NewMongoService(mongoURI, "mail_vault_e2e", "payloads")
	if err != nil {
		t.Fatalf("Mongo connection failure: %v", err)
	}

	t.Cleanup(func() {
		rStore.Close()
		dStore.Close()

		rawPG, _ := sql.Open("postgres", pgConnStr)
		defer rawPG.Close()
		_, _ = rawPG.Exec("DELETE FROM users WHERE username = 'grpcuser@test.local'")
		_, _ = rawPG.Exec("DELETE FROM email_meta WHERE recipient = 'grpcuser@test.local'")

		client, _ := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
		defer client.Disconnect(context.Background())
		_ = client.Database("mail_vault_e2e").Drop(context.Background())
	})

	// 1. Seed base user in DB so foreign key checks match
	rawPG, _ := sql.Open("postgres", pgConnStr)
	_, _ = rawPG.Exec("INSERT INTO users (username, password) VALUES ($1, $2)", "grpcuser@test.local", "secretpass")
	rawPG.Close()

	// 2. Spin up both servers
	grpcAddr, stopGrpc := startTestGrpcServer(t, rStore)
	defer stopGrpc()

	smtpAddr, stopSmtp := startTestSMTPServer(t, rStore, dStore)
	defer stopSmtp()

	// 3. USE GRPC CLIENT TO INJECT FILTER RULE
	conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial gRPC server: %v", err)
	}
	defer conn.Close()

	client := pb.NewRuleServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.AddRule(ctx, &pb.AddRuleRequest{
		Username: "grpcuser@test.local",
		Rule: &pb.Rule{
			Field:       "body",
			Operator:    "contains",
			Value:       "invoice",
			Action:      "move_to",
			ActionValue: "Finance",
		},
	})
	if err != nil {
		t.Fatalf("gRPC rule mutation rejected: %v", err)
	}

	// 4. Send email through SMTP and evaluate live execution
	err = sendMockEmail(t, smtpAddr, "billing@vendor.com", "grpcuser@test.local", "Invoice Target", "See attached corporate invoice.")
	if err != nil {
		t.Fatalf("SMTP transit failure: %v", err)
	}

	time.Sleep(150 * time.Millisecond) // flush wait

	// 5. Assert database records processed correctly
	pgAssert, _ := sql.Open("postgres", pgConnStr)
	defer pgAssert.Close()

	var folder, mongoDocID string
	err = pgAssert.QueryRow("SELECT folder, mongo_doc_id FROM email_meta WHERE recipient = 'grpcuser@test.local'").Scan(&folder, &mongoDocID)
	if err != nil {
		t.Fatalf("Relational verification tracking failed: %v", err)
	}

	if folder != "Finance" {
		t.Errorf("Rule processing engine mismatch. Expected 'Finance', got: %s", folder)
	}
}

func TestHighLoadSMTPLoading(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance profiling.")
	}

	rStore, _ := db.NewPostgresService(pgConnStr)
	dStore, _ := db.NewMongoService(mongoURI, "mail_vault_perf", "payloads")

	t.Cleanup(func() {
		rStore.Close()
		dStore.Close()
		rawPG, _ := sql.Open("postgres", pgConnStr)
		defer rawPG.Close()
		_, _ = rawPG.Exec("DELETE FROM users WHERE username = 'stressuser@test.local'")
		_, _ = rawPG.Exec("DELETE FROM email_meta WHERE recipient = 'stressuser@test.local'")

		client, _ := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
		defer client.Disconnect(context.Background())
		_ = client.Database("mail_vault_perf").Drop(context.Background())
	})

	rawPG, _ := sql.Open("postgres", pgConnStr)
	_, _ = rawPG.Exec("INSERT INTO users (username, password) VALUES ($1, $2)", "stressuser@test.local", "pass")
	rawPG.Close()

	smtpAddr, stopSmtp := startTestSMTPServer(t, rStore, dStore)
	defer stopSmtp()

	const totalMails = 200
	const concurrentWorkers = 20

	var wg sync.WaitGroup
	jobs := make(chan int, totalMails)

	startTime := time.Now()

	for i := 0; i < concurrentWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				randomBytes := make([]byte, 8)
				_, _ = rand.Read(randomBytes)
				uniqueToken := hex.EncodeToString(randomBytes)

				body := fmt.Sprintf("Performance transaction packet token: %s", uniqueToken)
				_ = sendMockEmail(t, smtpAddr, "generator@perf.io", "stressuser@test.local", "Load Test", body)
			}
		}()
	}

	for i := 0; i < totalMails; i++ {
		jobs <- i
	}
	close(jobs)

	wg.Wait()
	elapsed := time.Since(startTime)

	t.Logf("STRESS PROFILE COMPLETION: %d emails across %d threads in %v.", totalMails, concurrentWorkers, elapsed)
	t.Logf("THROUGHPUT METRIC: ~%.2f emails/second.", float64(totalMails)/elapsed.Seconds())
}
