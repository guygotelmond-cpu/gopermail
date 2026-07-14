import smtplib, time

with smtplib.SMTP('localhost', 1025) as s:
    s.ehlo()
    s.login('guygo@gmail.com', 'zxcdsa123')
    s.sendmail(
        'test-sender@external.com',
        ['guygo@gmail.com'],
        f"From: test-sender@external.com\r\nTo: guygo@gmail.com\r\nSubject: SSE Test {time.strftime('%H:%M:%S')}\r\n\r\nThis appeared instantly via SSE!\r\n"
    )
    print("sent!")