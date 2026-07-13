// Command controld runs the gRPC control-plane service used to mutate
// filter rules (currently AddRule). It is the machine-to-machine facing
// service — apiserver's REST layer is the human/browser-facing counterpart.
package main

import (
	"log"
	"net"
	"os"

	"gomail.com/db"
	pb "gomail.com/proto/control"

	"google.golang.org/grpc"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	pgConn := getenv("POSTGRES_DSN", "postgres://smtp_user:smtp_password@127.0.0.1:5432/smtp_metadata?sslmode=disable")
	addr := getenv("CONTROL_GRPC_ADDR", "0.0.0.0:9090")

	rStore, err := db.NewPostgresService(pgConn)
	if err != nil {
		log.Fatalf("CRITICAL: Postgres connection failure: %v", err)
	}
	defer rStore.Close()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to bind gRPC listener on %s: %v", addr, err)
	}

	grpcServer := grpc.NewServer()
	ruleServer := &pb.RuleGrpcServer{RStore: rStore}
	pb.RegisterRuleServiceServer(grpcServer, ruleServer)

	log.Printf("[controld] Rule control-plane gRPC service listening on %s...", addr)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("gRPC server crash: %v", err)
	}
}
