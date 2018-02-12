package muxstream

import (
	"fmt"
	"net"
	"sync"
	"testing"
)

type ServerSession interface {
	AcceptStream() (net.Conn, error)
}

type ClientSession interface {
	OpenStream() (net.Conn, error)
}

func setupTcpConn(tb testing.TB) (sConn, cConn net.Conn) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		tb.Fatal(err)
	}
	ch := make(chan net.Conn, 1)
	go func() {
		if conn, err := listener.Accept(); err != nil {
			tb.Fatal(err)
		} else {
			ch <- conn
		}
	}()

	addr := listener.Addr().String()
	cConn, err = net.Dial("tcp", addr)
	if err != nil {
		tb.Fatal(err)
	}
	sConn = <-ch
	return
}

func setupSession(tb testing.TB, sConf, cConf *Config) (
	server *Session, client *Session) {
	sConn, cConn := setupTcpConn(tb)
	server, _ = NewSession(sConn, sConf)
	client, _ = NewSession(cConn, cConf)
	return
}

func setupStream(tb testing.TB, server, client *Session) (cs, ss *Stream) {
	var err error
	ch := make(chan *Stream, 1)
	go func() {
		if stream, err := server.AcceptStream(); err != nil {
			tb.Fatal(err)
		} else {
			ch <- stream
		}
	}()
	cs, err = client.OpenStream()
	if err != nil {
		tb.Fatal(err)
	}
	ss = <-ch
	return
}

func echoServer(tb testing.TB, stream net.Conn) {
	p := make([]byte, 1024, 1024)
	for {
		if n, err := stream.Read(p); err != nil {
			if err != ERR_CH_REQ_WAS_CLOSED && err != ERR_CLOSED_STREAM {
				tb.Fatal("server:", err)
			}
			break
		} else if n, err = stream.Write(p[:n]); err != nil {
			tb.Fatal("server:", err)
		}
	}
}

func TestEcho(t *testing.T) {
	server, client := setupSession(t,
		(&Config{}).SetServer(), (&Config{}).SetClient())
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		stream, err := server.AcceptStream()
		if err != nil {
			t.Fatal(err)
		}
		echoServer(t, stream)

		stream.Close()
		server.Close()
	}()

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
				t.Fatal("client:", err)
			} else if n, err := stream.Read(p); err != nil {
				t.Fatal(err)
			} else if string(p[:n]) != msg {
				t.Fatalf("client: error, expect: %s, revice: %s",
					msg, string(p[:n]))
			}
		}

		stream.Close()
		client.Close()
	}()

	wg.Wait()
}

func TestParallel(t *testing.T) {
	server, client := setupSession(t,
		(&Config{}).SetServer(), (&Config{}).SetClient())
	pnum := 100
	rnum := 100

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < pnum; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				stream, err := server.AcceptStream()
				if err != nil {
					fmt.Println(err)
					t.Fatal(err)
				}
				echoServer(t, stream)
				stream.Close()
			}()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < pnum; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()

				stream, err := client.OpenStream()
				if err != nil {
					t.Fatal(err)
				}

				p := make([]byte, 1024, 1024)
				for i := 0; i < rnum; i++ {
					msg := fmt.Sprintf("(%d:%d)", stream.streamID, i)
					if _, err := stream.Write([]byte(msg)); err != nil {
						t.Fatal("client:", err)
					} else if n, err := stream.Read(p); err != nil {
						t.Fatal(err)
					} else if string(p[:n]) != msg {
						t.Fatal("client: error, expect: %s, revice: %s",
							msg, string(p[:n]))
					}
				}
				stream.Close()
			}(i)
		}
	}()

	wg.Wait()
	server.Close()
	client.Close()
}

func readAll(tb testing.TB, conn net.Conn, buf []byte, size int) (
	int, error) {
	i := 0
	for i < size {
		n, err := conn.Read(buf)
		if err != nil {
			tb.Fatal(err)
		}
		i += n
	}
	return size, nil
}

func simpleRW(tb testing.TB, conn0, conn1 net.Conn,
	buf0, buf1 []byte) {
	go func() {
		if n, err := readAll(tb, conn0, buf0, len(buf0)); err != nil {
			tb.Fatal(err)
		} else {
			if n, err = conn0.Write(buf0[:n]); err != nil {
				tb.Fatal(err)
			}
		}
	}()

	if n, err := conn1.Write(buf1); err != nil {
		tb.Fatal(err)
	} else {
		if n, err = readAll(tb, conn1, buf1[:n], n); err != nil {
			tb.Fatal(err)
		}
	}
}

func benchmarkMuxSpeed(b *testing.B, size int) {
	server, client := setupSession(b,
		(&Config{}).SetServer(), (&Config{}).SetClient())
	ss, cs := setupStream(b, server, client)

	buf0 := make([]byte, size, size)
	buf1 := make([]byte, size, size)
	for i := 0; i < b.N; i++ {
		simpleRW(b, ss, cs, buf0, buf1)
	}
}

func benchmarkTcpSpeed(b *testing.B, size int) {
	sConn, cConn := setupTcpConn(b)
	buf0 := make([]byte, size, size)
	buf1 := make([]byte, size, size)
	for i := 0; i < b.N; i++ {
		simpleRW(b, sConn, cConn, buf0, buf1)
	}
}

func BenchmarkMuxSpeed32K(b *testing.B) {
	benchmarkMuxSpeed(b, 32*1024)
}

func BenchmarkMuxSpeed16M(b *testing.B) {
	benchmarkMuxSpeed(b, 16*1024*1024)
}

func BenchmarkTcpSpeed32K(b *testing.B) {
	benchmarkTcpSpeed(b, 32*1024)
}

func BenchmarkTcpSpeed16M(b *testing.B) {
	benchmarkTcpSpeed(b, 16*1024*1024)
}
