package muxstream

import (
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/xtaci/smux"
)

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
	server.Close()
	client.Close()
}

func benchmarkTcpSpeed(b *testing.B, size int) {
	sConn, cConn := setupTcpConn(b)
	buf0 := make([]byte, size, size)
	buf1 := make([]byte, size, size)
	for i := 0; i < b.N; i++ {
		simpleRW(b, sConn, cConn, buf0, buf1)
	}
	sConn.Close()
	cConn.Close()
}

func benchmarkSmuxSpeed(b *testing.B, size int) {
	sConn, cConn := setupTcpConn(b)
	conf := &smux.Config{
		KeepAliveInterval: 10 * time.Second,
		KeepAliveTimeout:  30 * time.Second,
		MaxFrameSize:      65000,
		MaxReceiveBuffer:  4194304,
	}
	server, _ := smux.Server(sConn, conf)
	client, _ := smux.Client(cConn, conf)

	ch := make(chan *smux.Stream, 1)
	go func() {
		if stream, err := server.AcceptStream(); err != nil {
			b.Fatal(err)
		} else {
			ch <- stream
		}
	}()
	cs, err := client.OpenStream()
	if err != nil {
		b.Fatal(err)
	}
	ss := <-ch

	buf0 := make([]byte, size, size)
	buf1 := make([]byte, size, size)
	for i := 0; i < b.N; i++ {
		simpleRW(b, ss, cs, buf0, buf1)
	}
	server.Close()
	client.Close()
}

func benchmarkYamuxSpeed(b *testing.B, size int) {
	sConn, cConn := setupTcpConn(b)
	server, _ := yamux.Server(sConn, nil)
	client, _ := yamux.Client(cConn, nil)

	ch := make(chan *yamux.Stream, 1)
	go func() {
		if stream, err := server.AcceptStream(); err != nil {
			b.Fatal(err)
		} else {
			ch <- stream
		}
	}()
	cs, err := client.OpenStream()
	if err != nil {
		b.Fatal(err)
	}
	ss := <-ch

	buf0 := make([]byte, size, size)
	buf1 := make([]byte, size, size)
	for i := 0; i < b.N; i++ {
		simpleRW(b, ss, cs, buf0, buf1)
	}
	server.Close()
	client.Close()
}

func BenchmarkMuxSpeed32K(b *testing.B) {
	benchmarkMuxSpeed(b, 32*1024)
}

func BenchmarkMuxSpeed16M(b *testing.B) {
	benchmarkMuxSpeed(b, 16*1024*1024)
}

func BenchmarkSmuxSpeed32K(b *testing.B) {
	benchmarkSmuxSpeed(b, 32*1024)
}

func BenchmarkSmuxSpeed16M(b *testing.B) {
	benchmarkSmuxSpeed(b, 16*1024*1024)
}

// func BenchmarkYamuxSpeed32K(b *testing.B) {
// 	benchmarkYamuxSpeed(b, 32*1024)
// }

// func BenchmarkYamuxSpeed16M(b *testing.B) {
// 	benchmarkYamuxSpeed(b, 16*1024*1024)
// }

// func BenchmarkTcpSpeed32K(b *testing.B) {
// 	benchmarkTcpSpeed(b, 32*1024)
// }

// func BenchmarkTcpSpeed16M(b *testing.B) {
// 	benchmarkTcpSpeed(b, 16*1024*1024)
// }
