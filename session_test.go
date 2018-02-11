package muxstream

import (
	"fmt"
	"net"
	"sync"
	"testing"
)

func setupConn(tb testing.TB) (sConn net.Conn, cConn net.Conn) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		tb.Fatal(err)
	}
	ch := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			tb.Fatal(err)
		}
		ch <- conn
	}()

	addr := listener.Addr().String()
	cConn, err = net.Dial("tcp", addr)
	if err != nil {
		tb.Fatal(err)
	}

	sConn = <-ch
	return
}

func TestEcho(t *testing.T) {
	var wg sync.WaitGroup
	sConn, cConn := setupConn(t)

	server, err := NewSession(sConn, (&Config{}).SetServer())
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewSession(cConn, (&Config{}).SetClient())
	if err != nil {
		t.Fatal(err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		stream, err := server.AcceptStream()
		if err != nil {
			t.Fatal(err)
		}

		p := make([]byte, 1024, 1024)
		for {
			if n, err := stream.Read(p); err != nil {
				t.Log("server: ", err)
				break
			} else if n, err = stream.Write(p[:n]); err != nil {
				t.Log("server: ", err)
				break
			}
		}

		stream.Close()
		server.Close()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		stream, err := client.OpenStream()
		if err != nil {
			t.Fatal(err)
		}

		p := make([]byte, 1024, 1024)
		for i := 0; i < 10; i++ {
			msg := fmt.Sprintf("%d: hello world!", i)
			if _, err := stream.Write([]byte(msg)); err != nil {
				t.Fatal("client: ", err)
			} else if n, err := stream.Read(p); err != nil {
				t.Fatal(err)
			} else {
				t.Log("client: ", string(p[:n]))
			}
		}

		stream.Close()
		client.Close()
	}()

	wg.Wait()
}
