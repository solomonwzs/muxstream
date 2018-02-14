package muxstream

import (
	"bytes"
	crand "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

type slowNetConn struct {
	net.Conn
	delay time.Duration
}

func (conn *slowNetConn) Read(p []byte) (int, error) {
	time.Sleep(conn.delay)
	return conn.Conn.Read(p)
}

func (conn *slowNetConn) Write(p []byte) (int, error) {
	time.Sleep(conn.delay)
	return conn.Conn.Write(p)
}

func drawProgressBar(n, m int, len int) {
	nLen := n * len / m
	buf := new(bytes.Buffer)
	buf.WriteByte('[')
	for i := 0; i < nLen; i++ {
		buf.WriteByte('#')
	}
	for i := 0; i < len-nLen; i++ {
		buf.WriteByte('-')
	}
	buf.WriteByte(']')
	buf.WriteString(fmt.Sprintf(" %.2f%%\r", float64(n)/float64(m)*100))

	os.Stdout.Write(buf.Bytes())
	os.Stdout.Sync()
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

func simpleServer(tb testing.TB, session *Session) {
	for {
		if stream, err := session.AcceptStream(); err == nil {
			go echoServer(tb, stream)
		} else {
			if err != ERR_CH_REQ_WAS_CLOSED &&
				err != ERR_SESSION_WAS_CLOSED {
				tb.Error(err)
			}
			return
		}
	}
}

func TestBasic(t *testing.T) {
	server, client := setupSession(t,
		(&Config{}).SetServer(), (&Config{}).SetClient())
	ss, cs := setupStream(t, server, client)

	p0 := []byte("0123456789")
	if _, err := cs.Write(p0[:5]); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.Write(p0[5:]); err != nil {
		t.Fatal(err)
	}
	cs.Close()
	time.Sleep(10 * time.Millisecond)

	p1 := make([]byte, 1024, 1024)
	if n, err := ss.Read(p1); err != nil {
		t.Fatal(err)
	} else if string(p0) != string(p1[:n]) {
		t.Fatal("error response")
	}

	server.Close()
	client.Close()
}

func TestEcho(t *testing.T) {
	server, client := setupSession(t,
		(&Config{}).SetServer(), (&Config{}).SetClient())
	ss, cs := setupStream(t, server, client)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		echoServer(t, ss)
		ss.Close()
		server.Close()
	}()

	go func() {
		defer wg.Done()

		p := make([]byte, 1024, 1024)
		for i := 0; i < 10; i++ {
			msg := fmt.Sprintf("%d: hello world!", i)
			if _, err := cs.Write([]byte(msg)); err != nil {
				t.Fatal("client:", err)
			} else if n, err := cs.Read(p); err != nil {
				t.Fatal(err)
			} else if string(p[:n]) != msg {
				t.Fatalf("client: error, expect: %s, revice: %s",
					msg, string(p[:n]))
			}
		}
		cs.Close()
		client.Close()
	}()

	wg.Wait()
}

func TestParallel(t *testing.T) {
	server, client := setupSession(t,
		(&Config{}).SetServer(), (&Config{}).SetClient())
	pnum := 300
	rnum := 300

	var wgCount sync.WaitGroup
	countCh := make(chan bool, pnum)
	wgCount.Add(1)
	go func() {
		defer wgCount.Done()
		n := 0
		m := pnum * rnum / 100
		for range countCh {
			n += 1
			if n%m == 0 {
				drawProgressBar(n, pnum*rnum, 60)
			}
		}
		drawProgressBar(1, 1, 60)
		fmt.Printf("\n")
	}()

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
					countCh <- true
				}
				stream.Close()
			}(i)
		}
	}()

	wg.Wait()
	server.Close()
	client.Close()

	close(countCh)
	wgCount.Wait()
}

func TestRandomData(t *testing.T) {
	server, client := setupSession(t,
		(&Config{}).SetServer(), (&Config{}).SetClient())
	go simpleServer(t, server)

	for i := 0; i < 100; i++ {
		rnd := make([]byte, rand.Uint32()%1024)
		io.ReadFull(crand.Reader, rnd)
		client.rwc.Write(rnd)
	}
	client.Close()
	server.Close()
}

func TestReadDeadline(t *testing.T) {
	server, client := setupSession(t,
		(&Config{}).SetServer(), (&Config{}).SetClient())
	go simpleServer(t, server)

	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			stream, err := client.OpenStream()
			if err != nil {
				t.Fatal(err)
			}
			p := make([]byte, 16)
			stream.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			if _, err = stream.Read(p); err != ERR_STREAM_IO_TIMEOUT {
				t.Fatal("set read deadline fail")
			}
		}()
	}
	wg.Wait()
}

func TestWriteDeadline(t *testing.T) {
	delay := 40 * time.Millisecond
	sConn, cConn0 := setupTcpConn(t)
	cConn := &slowNetConn{cConn0, 2 * delay}
	server, _ := NewSession(sConn, (&Config{}).SetServer())
	client, _ := NewSession(cConn, (&Config{}).SetClient())
	go simpleServer(t, server)

	var wg sync.WaitGroup
	n := 10
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			stream, err := client.OpenStream()
			if err != nil {
				t.Fatal(err)
			}
			p := make([]byte, 16)
			stream.SetWriteDeadline(time.Now().Add(delay))
			if _, err = stream.Write(p); err != ERR_STREAM_IO_TIMEOUT {
				fmt.Println(err)
				t.Fatal("set write deadline fail")
			}
		}()
	}
	wg.Wait()
}
