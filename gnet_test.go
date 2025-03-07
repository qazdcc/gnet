// Copyright (c) 2019 Andy Pan
// Copyright (c) 2017 Joshua J Baker
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gnet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"net"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	gerr "github.com/panjf2000/gnet/v2/pkg/errors"
	"github.com/panjf2000/gnet/v2/pkg/logging"
	bbPool "github.com/panjf2000/gnet/v2/pkg/pool/bytebuffer"
	goPool "github.com/panjf2000/gnet/v2/pkg/pool/goroutine"
)

var streamLen = 1024 * 1024

func TestServe(t *testing.T) {
	// start an engine
	// connect 10 clients
	// each client will pipe random data for 1-3 seconds.
	// the writes to the engine will be random sizes. 0KB - 1MB.
	// the engine will echo back the data.
	// waits for graceful connection closing.
	t.Run("poll", func(t *testing.T) {
		t.Run("tcp", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9991", false, false, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9992", false, false, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("tcp-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9991", false, false, false, true, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9992", false, false, true, true, false, 10, LeastConnections)
			})
		})
		t.Run("tcp-async-writev", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9991", false, false, false, true, true, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9992", false, false, true, true, true, 10, LeastConnections)
			})
		})
		t.Run("udp", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "udp", ":9991", false, false, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "udp", ":9992", false, false, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("udp-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "udp", ":9991", false, false, false, true, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "udp", ":9992", false, false, true, true, false, 10, LeastConnections)
			})
		})
		t.Run("unix", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet1.sock", false, false, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet2.sock", false, false, true, false, false, 10, SourceAddrHash)
			})
		})
		t.Run("unix-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet1.sock", false, false, false, true, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet2.sock", false, false, true, true, false, 10, SourceAddrHash)
			})
		})
		t.Run("unix-async-writev", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet1.sock", false, false, false, true, true, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet2.sock", false, false, true, true, true, 10, SourceAddrHash)
			})
		})
	})

	t.Run("poll-reuseport", func(t *testing.T) {
		t.Run("tcp", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9991", true, false, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9992", true, false, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("tcp-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9991", true, false, false, true, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9992", true, false, true, true, false, 10, LeastConnections)
			})
		})
		t.Run("tcp-async-writev", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9991", true, false, false, true, true, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9992", true, false, true, true, true, 10, LeastConnections)
			})
		})
		t.Run("udp", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "udp", ":9991", true, false, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "udp", ":9992", true, false, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("udp-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "udp", ":9991", true, false, false, true, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "udp", ":9992", true, false, true, true, false, 10, LeastConnections)
			})
		})
		t.Run("unix", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet1.sock", true, false, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet2.sock", true, false, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("unix-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet1.sock", true, false, false, true, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet2.sock", true, false, true, true, false, 10, LeastConnections)
			})
		})
		t.Run("unix-async-writev", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet1.sock", true, false, false, true, true, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet2.sock", true, false, true, true, true, 10, LeastConnections)
			})
		})
	})

	t.Run("poll-reuseaddr", func(t *testing.T) {
		t.Run("tcp", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9991", false, true, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9992", false, true, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("tcp-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9991", false, true, false, true, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "tcp", ":9992", false, true, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("udp", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "udp", ":9991", false, true, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "udp", ":9992", false, true, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("udp-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "udp", ":9991", false, true, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "udp", ":9992", false, true, true, true, false, 10, LeastConnections)
			})
		})
		t.Run("unix", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet1.sock", false, true, false, false, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet2.sock", false, true, true, false, false, 10, LeastConnections)
			})
		})
		t.Run("unix-async", func(t *testing.T) {
			t.Run("1-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet1.sock", false, true, false, true, false, 10, RoundRobin)
			})
			t.Run("N-loop", func(t *testing.T) {
				testServe(t, "unix", "gnet2.sock", false, true, true, true, false, 10, LeastConnections)
			})
		})
	})
}

type testServer struct {
	*BuiltinEventEngine
	tester       *testing.T
	eng          Engine
	network      string
	addr         string
	multicore    bool
	async        bool
	writev       bool
	nclients     int
	started      int32
	connected    int32
	clientActive int32
	disconnected int32
	workerPool   *goPool.Pool
}

func (s *testServer) OnBoot(eng Engine) (action Action) {
	s.eng = eng
	return
}

func (s *testServer) OnOpen(c Conn) (out []byte, action Action) {
	c.SetContext(c)
	atomic.AddInt32(&s.connected, 1)
	out = []byte("sweetness\r\n")
	require.NotNil(s.tester, c.LocalAddr(), "nil local addr")
	require.NotNil(s.tester, c.RemoteAddr(), "nil remote addr")
	return
}

func (s *testServer) OnClose(c Conn, err error) (action Action) {
	if err != nil {
		logging.Debugf("error occurred on closed, %v\n", err)
	}
	if s.network != "udp" {
		require.Equal(s.tester, c.Context(), c, "invalid context")
	}

	atomic.AddInt32(&s.disconnected, 1)
	if atomic.LoadInt32(&s.connected) == atomic.LoadInt32(&s.disconnected) &&
		atomic.LoadInt32(&s.disconnected) == int32(s.nclients) {
		action = Shutdown
		s.workerPool.Release()
	}

	return
}

func (s *testServer) OnTraffic(c Conn) (action Action) {
	if s.async {
		buf := bbPool.Get()
		_, _ = c.WriteTo(buf)

		if s.network == "tcp" || s.network == "unix" {
			// just for test
			_ = c.InboundBuffered()
			_ = c.OutboundBuffered()
			_, _ = c.Discard(1)

			_ = s.workerPool.Submit(
				func() {
					if s.writev {
						mid := buf.Len() / 2
						bs := make([][]byte, 2)
						bs[0] = buf.B[:mid]
						bs[1] = buf.B[mid:]
						_ = c.AsyncWritev(bs)
					} else {
						_ = c.AsyncWrite(buf.Bytes())
					}
				})
			return
		} else if s.network == "udp" {
			_ = s.workerPool.Submit(
				func() {
					_ = c.AsyncWrite(buf.Bytes())
				})
			return
		}
		return
	}
	buf, _ := c.Next(-1)
	_, _ = c.Write(buf)
	return
}

func (s *testServer) OnTick() (delay time.Duration, action Action) {
	if atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		for i := 0; i < s.nclients; i++ {
			atomic.AddInt32(&s.clientActive, 1)
			go func() {
				startClient(s.tester, s.network, s.addr, s.multicore, s.async)
				atomic.AddInt32(&s.clientActive, -1)
			}()
		}
	}
	if s.network == "udp" && atomic.LoadInt32(&s.clientActive) == 0 {
		action = Shutdown
		return
	}
	delay = time.Second / 5
	return
}

func testServe(t *testing.T, network, addr string, reuseport, reuseaddr, multicore, async, writev bool, nclients int, lb LoadBalancing) {
	ts := &testServer{
		tester:     t,
		network:    network,
		addr:       addr,
		multicore:  multicore,
		async:      async,
		writev:     writev,
		nclients:   nclients,
		workerPool: goPool.Default(),
	}
	err := Run(ts,
		network+"://"+addr,
		WithLockOSThread(async),
		WithMulticore(multicore),
		WithReusePort(reuseport),
		WithReuseAddr(reuseaddr),
		WithTicker(true),
		WithTCPKeepAlive(time.Minute*1),
		WithTCPNoDelay(TCPDelay),
		WithLoadBalancing(lb))
	assert.NoError(t, err)
}

func startClient(t *testing.T, network, addr string, multicore, async bool) {
	rand.Seed(time.Now().UnixNano())
	c, err := net.Dial(network, addr)
	require.NoError(t, err)
	defer c.Close()
	rd := bufio.NewReader(c)
	if network != "udp" {
		msg, err := rd.ReadBytes('\n')
		require.NoError(t, err)
		require.Equal(t, string(msg), "sweetness\r\n", "bad header")
	}
	duration := time.Duration((rand.Float64()*2+1)*float64(time.Second)) / 2
	t.Logf("test duration: %dms", duration/time.Millisecond)
	start := time.Now()
	for time.Since(start) < duration {
		reqData := make([]byte, streamLen)
		if network == "udp" {
			reqData = reqData[:1024]
		}
		_, err = rand.Read(reqData)
		require.NoError(t, err)
		_, err = c.Write(reqData)
		require.NoError(t, err)
		respData := make([]byte, len(reqData))
		_, err = io.ReadFull(rd, respData)
		require.NoError(t, err)
		if !async {
			// require.Equalf(t, reqData, respData, "response mismatch with protocol:%s, multi-core:%t, content of bytes: %d vs %d", network, multicore, string(reqData), string(respData))
			require.Equalf(
				t,
				reqData,
				respData,
				"response mismatch with protocol:%s, multi-core:%t, length of bytes: %d vs %d",
				network,
				multicore,
				len(reqData),
				len(respData),
			)
		}
	}
}

func TestDefaultGnetServer(t *testing.T) {
	svr := BuiltinEventEngine{}
	svr.OnBoot(Engine{})
	svr.OnOpen(nil)
	svr.OnClose(nil, nil)
	svr.OnTraffic(nil)
	svr.OnTick()
}

type testBadAddrServer struct {
	*BuiltinEventEngine
}

func (t *testBadAddrServer) OnBoot(_ Engine) (action Action) {
	return Shutdown
}

func TestBadAddresses(t *testing.T) {
	events := new(testBadAddrServer)
	err := Run(events, "tulip://howdy")
	assert.Error(t, err)
	err = Run(events, "howdy")
	assert.Error(t, err)
	err = Run(events, "tcp://")
	assert.NoError(t, err)
}

func TestTick(t *testing.T) {
	testTick("tcp", ":9989", t)
}

type testTickServer struct {
	*BuiltinEventEngine
	count int
}

func (t *testTickServer) OnTick() (delay time.Duration, action Action) {
	if t.count == 25 {
		action = Shutdown
		return
	}
	t.count++
	delay = time.Millisecond * 10
	return
}

func testTick(network, addr string, t *testing.T) {
	events := &testTickServer{}
	start := time.Now()
	opts := Options{Ticker: true}
	err := Run(events, network+"://"+addr, WithOptions(opts))
	assert.NoError(t, err)
	dur := time.Since(start)
	if dur < 250&time.Millisecond || dur > time.Second {
		t.Logf("bad ticker timing: %d", dur)
	}
}

func TestWakeConn(t *testing.T) {
	testWakeConn(t, "tcp", ":9990")
}

type testWakeConnServer struct {
	*BuiltinEventEngine
	tester  *testing.T
	network string
	addr    string
	conn    chan Conn
	c       Conn
	wake    bool
}

func (t *testWakeConnServer) OnOpen(c Conn) (out []byte, action Action) {
	t.conn <- c
	return
}

func (t *testWakeConnServer) OnClose(c Conn, err error) (action Action) {
	action = Shutdown
	return
}

func (t *testWakeConnServer) OnTraffic(c Conn) (action Action) {
	_, _ = c.Write([]byte("Waking up."))
	action = -1
	return
}

func (t *testWakeConnServer) OnTick() (delay time.Duration, action Action) {
	if !t.wake {
		t.wake = true
		delay = time.Millisecond * 100
		go func() {
			conn, err := net.Dial(t.network, t.addr)
			require.NoError(t.tester, err)
			defer conn.Close()
			r := make([]byte, 10)
			_, err = conn.Read(r)
			require.NoError(t.tester, err)
		}()
		return
	}
	t.c = <-t.conn
	_ = t.c.Wake()
	delay = time.Millisecond * 100
	return
}

func testWakeConn(t *testing.T, network, addr string) {
	svr := &testWakeConnServer{tester: t, network: network, addr: addr, conn: make(chan Conn, 1)}
	logger := zap.NewExample()
	err := Run(svr, network+"://"+addr, WithTicker(true), WithNumEventLoop(2*runtime.NumCPU()),
		WithLogger(logger.Sugar()))
	assert.NoError(t, err)
	_ = logger.Sync()
}

func TestShutdown(t *testing.T) {
	testShutdown(t, "tcp", ":9991")
}

type testShutdownServer struct {
	*BuiltinEventEngine
	tester  *testing.T
	network string
	addr    string
	count   int
	clients int64
	N       int
}

func (t *testShutdownServer) OnOpen(c Conn) (out []byte, action Action) {
	atomic.AddInt64(&t.clients, 1)
	return
}

func (t *testShutdownServer) OnClose(c Conn, err error) (action Action) {
	atomic.AddInt64(&t.clients, -1)
	return
}

func (t *testShutdownServer) OnTick() (delay time.Duration, action Action) {
	if t.count == 0 {
		// start clients
		for i := 0; i < t.N; i++ {
			go func() {
				conn, err := net.Dial(t.network, t.addr)
				require.NoError(t.tester, err)
				defer conn.Close()
				_, err = conn.Read([]byte{0})
				require.Error(t.tester, err)
			}()
		}
	} else if int(atomic.LoadInt64(&t.clients)) == t.N {
		action = Shutdown
	}
	t.count++
	delay = time.Second / 20
	return
}

func testShutdown(t *testing.T, network, addr string) {
	events := &testShutdownServer{tester: t, network: network, addr: addr, N: 10}
	err := Run(events, network+"://"+addr, WithTicker(true))
	assert.NoError(t, err)
	require.Equal(t, int(events.clients), 0, "did not call close on all clients")
}

func TestCloseActionError(t *testing.T) {
	testCloseActionError(t, "tcp", ":9992")
}

type testCloseActionErrorServer struct {
	*BuiltinEventEngine
	tester        *testing.T
	network, addr string
	action        bool
}

func (t *testCloseActionErrorServer) OnClose(c Conn, err error) (action Action) {
	action = Shutdown
	return
}

func (t *testCloseActionErrorServer) OnTraffic(c Conn) (action Action) {
	n := c.InboundBuffered()
	buf, _ := c.Peek(n)
	_, _ = c.Write(buf)
	_, _ = c.Discard(n)
	action = Close
	return
}

func (t *testCloseActionErrorServer) OnTick() (delay time.Duration, action Action) {
	if !t.action {
		t.action = true
		delay = time.Millisecond * 100
		go func() {
			conn, err := net.Dial(t.network, t.addr)
			require.NoError(t.tester, err)
			defer conn.Close()
			data := []byte("Hello World!")
			_, _ = conn.Write(data)
			_, err = conn.Read(data)
			require.NoError(t.tester, err)
		}()
		return
	}
	delay = time.Millisecond * 100
	return
}

func testCloseActionError(t *testing.T, network, addr string) {
	events := &testCloseActionErrorServer{tester: t, network: network, addr: addr}
	err := Run(events, network+"://"+addr, WithTicker(true))
	assert.NoError(t, err)
}

func TestShutdownActionError(t *testing.T) {
	testShutdownActionError(t, "tcp", ":9993")
}

type testShutdownActionErrorServer struct {
	*BuiltinEventEngine
	tester        *testing.T
	network, addr string
	action        bool
}

func (t *testShutdownActionErrorServer) OnTraffic(c Conn) (action Action) {
	buf, _ := c.Peek(-1)
	_, _ = c.Write(buf)
	_, _ = c.Discard(-1)
	action = Shutdown
	return
}

func (t *testShutdownActionErrorServer) OnTick() (delay time.Duration, action Action) {
	if !t.action {
		t.action = true
		delay = time.Millisecond * 100
		go func() {
			conn, err := net.Dial(t.network, t.addr)
			require.NoError(t.tester, err)
			defer conn.Close()
			data := []byte("Hello World!")
			_, _ = conn.Write(data)
			_, err = conn.Read(data)
			require.NoError(t.tester, err)
		}()
		return
	}
	delay = time.Millisecond * 100
	return
}

func testShutdownActionError(t *testing.T, network, addr string) {
	events := &testShutdownActionErrorServer{tester: t, network: network, addr: addr}
	err := Run(events, network+"://"+addr, WithTicker(true))
	assert.NoError(t, err)
}

func TestCloseActionOnOpen(t *testing.T) {
	testCloseActionOnOpen(t, "tcp", ":9994")
}

type testCloseActionOnOpenServer struct {
	*BuiltinEventEngine
	tester        *testing.T
	network, addr string
	action        bool
}

func (t *testCloseActionOnOpenServer) OnOpen(c Conn) (out []byte, action Action) {
	action = Close
	return
}

func (t *testCloseActionOnOpenServer) OnClose(c Conn, err error) (action Action) {
	action = Shutdown
	return
}

func (t *testCloseActionOnOpenServer) OnTick() (delay time.Duration, action Action) {
	if !t.action {
		t.action = true
		delay = time.Millisecond * 100
		go func() {
			conn, err := net.Dial(t.network, t.addr)
			require.NoError(t.tester, err)
			defer conn.Close()
		}()
		return
	}
	delay = time.Millisecond * 100
	return
}

func testCloseActionOnOpen(t *testing.T, network, addr string) {
	events := &testCloseActionOnOpenServer{tester: t, network: network, addr: addr}
	err := Run(events, network+"://"+addr, WithTicker(true))
	assert.NoError(t, err)
}

func TestShutdownActionOnOpen(t *testing.T) {
	testShutdownActionOnOpen(t, "tcp", ":9995")
}

type testShutdownActionOnOpenServer struct {
	*BuiltinEventEngine
	tester        *testing.T
	network, addr string
	action        bool
}

func (t *testShutdownActionOnOpenServer) OnOpen(c Conn) (out []byte, action Action) {
	action = Shutdown
	return
}

func (t *testShutdownActionOnOpenServer) OnShutdown(s Engine) {
	dupFD, err := s.DupFd()
	logging.Debugf("dup fd: %d with error: %v\n", dupFD, err)
}

func (t *testShutdownActionOnOpenServer) OnTick() (delay time.Duration, action Action) {
	if !t.action {
		t.action = true
		delay = time.Millisecond * 100
		go func() {
			conn, err := net.Dial(t.network, t.addr)
			require.NoError(t.tester, err)
			defer conn.Close()
		}()
		return
	}
	delay = time.Millisecond * 100
	return
}

func testShutdownActionOnOpen(t *testing.T, network, addr string) {
	events := &testShutdownActionOnOpenServer{tester: t, network: network, addr: addr}
	err := Run(events, network+"://"+addr, WithTicker(true))
	assert.NoError(t, err)
}

func TestUDPShutdown(t *testing.T) {
	testUDPShutdown(t, "udp4", ":9000")
}

type testUDPShutdownServer struct {
	*BuiltinEventEngine
	tester  *testing.T
	network string
	addr    string
	tick    bool
}

func (t *testUDPShutdownServer) OnTraffic(c Conn) (action Action) {
	buf, _ := c.Peek(-1)
	_, _ = c.Write(buf)
	_, _ = c.Discard(-1)
	action = Shutdown
	return
}

func (t *testUDPShutdownServer) OnTick() (delay time.Duration, action Action) {
	if !t.tick {
		t.tick = true
		delay = time.Millisecond * 100
		go func() {
			conn, err := net.Dial(t.network, t.addr)
			require.NoError(t.tester, err)
			defer conn.Close()
			data := []byte("Hello World!")
			_, err = conn.Write(data)
			require.NoError(t.tester, err)
			_, err = conn.Read(data)
			require.NoError(t.tester, err)
		}()
		return
	}
	delay = time.Millisecond * 100
	return
}

func testUDPShutdown(t *testing.T, network, addr string) {
	svr := &testUDPShutdownServer{tester: t, network: network, addr: addr}
	err := Run(svr, network+"://"+addr, WithTicker(true))
	assert.NoError(t, err)
}

func TestCloseConnection(t *testing.T) {
	testCloseConnection(t, "tcp", ":9996")
}

type testCloseConnectionServer struct {
	*BuiltinEventEngine
	tester        *testing.T
	network, addr string
	action        bool
}

func (t *testCloseConnectionServer) OnClose(c Conn, err error) (action Action) {
	action = Shutdown
	return
}

func (t *testCloseConnectionServer) OnTraffic(c Conn) (action Action) {
	buf, _ := c.Peek(-1)
	_, _ = c.Write(buf)
	_, _ = c.Discard(-1)
	go func() {
		time.Sleep(time.Second)
		_ = c.Close()
	}()
	return
}

func (t *testCloseConnectionServer) OnTick() (delay time.Duration, action Action) {
	delay = time.Millisecond * 100
	if !t.action {
		t.action = true
		go func() {
			conn, err := net.Dial(t.network, t.addr)
			require.NoError(t.tester, err)
			defer conn.Close()
			data := []byte("Hello World!")
			_, _ = conn.Write(data)
			_, err = conn.Read(data)
			require.NoError(t.tester, err)
			// waiting the engine shutdown.
			_, err = conn.Read(data)
			require.Error(t.tester, err)
		}()
		return
	}
	return
}

func testCloseConnection(t *testing.T, network, addr string) {
	events := &testCloseConnectionServer{tester: t, network: network, addr: addr}
	err := Run(events, network+"://"+addr, WithTicker(true))
	assert.NoError(t, err)
}

func TestServerOptionsCheck(t *testing.T) {
	err := Run(&BuiltinEventEngine{}, "tcp://:3500", WithNumEventLoop(10001), WithLockOSThread(true))
	assert.EqualError(t, err, gerr.ErrTooManyEventLoopThreads.Error(), "error returned with LockOSThread option")
}

func TestStop(t *testing.T) {
	testStop(t, "tcp", ":9997")
}

type testStopServer struct {
	*BuiltinEventEngine
	tester                   *testing.T
	network, addr, protoAddr string
	action                   bool
}

func (t *testStopServer) OnClose(c Conn, err error) (action Action) {
	logging.Debugf("closing connection...")
	return
}

func (t *testStopServer) OnTraffic(c Conn) (action Action) {
	buf, _ := c.Peek(-1)
	_, _ = c.Write(buf)
	_, _ = c.Discard(-1)
	return
}

func (t *testStopServer) OnTick() (delay time.Duration, action Action) {
	delay = time.Millisecond * 100
	if !t.action {
		t.action = true
		go func() {
			conn, err := net.Dial(t.network, t.addr)
			require.NoError(t.tester, err)
			defer conn.Close()
			data := []byte("Hello World!")
			_, _ = conn.Write(data)
			_, err = conn.Read(data)
			require.NoError(t.tester, err)

			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				logging.Debugf("stop engine...", Stop(ctx, t.protoAddr))
			}()

			// waiting the engine shutdown.
			_, err = conn.Read(data)
			require.Error(t.tester, err)
		}()
		return
	}
	return
}

func testStop(t *testing.T, network, addr string) {
	events := &testStopServer{tester: t, network: network, addr: addr, protoAddr: network + "://" + addr}
	err := Run(events, events.protoAddr, WithTicker(true))
	assert.NoError(t, err)
}

// Test should not panic when we wake-up server_closed conn.
func TestClosedWakeUp(t *testing.T) {
	events := &testClosedWakeUpServer{
		tester:             t,
		BuiltinEventEngine: &BuiltinEventEngine{}, network: "tcp", addr: ":8888", protoAddr: "tcp://:8888",
		clientClosed: make(chan struct{}),
		serverClosed: make(chan struct{}),
		wakeup:       make(chan struct{}),
	}

	err := Run(events, events.protoAddr)
	assert.NoError(t, err)
}

type testClosedWakeUpServer struct {
	*BuiltinEventEngine
	tester                   *testing.T
	network, addr, protoAddr string

	wakeup       chan struct{}
	serverClosed chan struct{}
	clientClosed chan struct{}
}

func (s *testClosedWakeUpServer) OnBoot(_ Engine) (action Action) {
	go func() {
		c, err := net.Dial(s.network, s.addr)
		require.NoError(s.tester, err)

		_, err = c.Write([]byte("hello"))
		require.NoError(s.tester, err)

		<-s.wakeup
		_, err = c.Write([]byte("hello again"))
		require.NoError(s.tester, err)

		close(s.clientClosed)
		<-s.serverClosed

		logging.Debugf("stop engine...", Stop(context.TODO(), s.protoAddr))
	}()

	return None
}

func (s *testClosedWakeUpServer) OnTraffic(c Conn) Action {
	require.NotNil(s.tester, c.RemoteAddr())

	select {
	case <-s.wakeup:
	default:
		close(s.wakeup)
	}

	go func() { require.NoError(s.tester, c.Wake()) }()
	go func() { require.NoError(s.tester, c.Close()) }()

	<-s.clientClosed

	_, _ = c.Write([]byte("answer"))
	return None
}

func (s *testClosedWakeUpServer) OnClose(c Conn, err error) (action Action) {
	select {
	case <-s.serverClosed:
	default:
		close(s.serverClosed)
	}
	return
}

var errIncompletePacket = errors.New("incomplete packet")

type simServer struct {
	BuiltinEventEngine
	tester       *testing.T
	eng          Engine
	network      string
	addr         string
	multicore    bool
	nclients     int
	packetSize   int
	packetBatch  int
	started      int32
	connected    int32
	disconnected int32
}

func (s *simServer) OnBoot(eng Engine) (action Action) {
	s.eng = eng
	return
}

func (s *simServer) OnOpen(c Conn) (out []byte, action Action) {
	c.SetContext(&testCodec{})
	atomic.AddInt32(&s.connected, 1)
	out = []byte("sweetness\r\n")
	require.NotNil(s.tester, c.LocalAddr(), "nil local addr")
	require.NotNil(s.tester, c.RemoteAddr(), "nil remote addr")
	return
}

func (s *simServer) OnClose(c Conn, err error) (action Action) {
	if err != nil {
		logging.Debugf("error occurred on closed, %v\n", err)
	}

	atomic.AddInt32(&s.disconnected, 1)
	if atomic.LoadInt32(&s.connected) == atomic.LoadInt32(&s.disconnected) &&
		atomic.LoadInt32(&s.disconnected) == int32(s.nclients) {
		action = Shutdown
	}

	return
}

func (s *simServer) OnTraffic(c Conn) (action Action) {
	codec := c.Context().(*testCodec)
	var packets [][]byte
	for {
		data, err := codec.Decode(c)
		if err == errIncompletePacket {
			break
		}
		if err != nil {
			logging.Errorf("invalid packet: %v", err)
			return Close
		}
		packet, _ := codec.Encode(data)
		codec.Discard(c)
		packets = append(packets, packet)
	}
	if n := len(packets); n > 1 {
		_, _ = c.Writev(packets)
	} else if n == 1 {
		_, _ = c.Write(packets[0])
	}
	return
}

func (s *simServer) OnTick() (delay time.Duration, action Action) {
	if atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		for i := 0; i < s.nclients; i++ {
			go func() {
				runClient(s.tester, s.network, s.addr, s.packetSize, s.packetBatch)
			}()
		}
	}
	delay = 100 * time.Millisecond
	return
}

// All current protocols.
const (
	magicNumber     = 1314
	magicNumberSize = 2
	bodySize        = 4
)

var magicNumberBytes []byte

func init() {
	magicNumberBytes = make([]byte, magicNumberSize)
	binary.BigEndian.PutUint16(magicNumberBytes, uint16(magicNumber))
}

// Protocol format:
//
// * 0           2                       6
// * +-----------+-----------------------+
// * |   magic   |       body len        |
// * +-----------+-----------+-----------+
// * |                                   |
// * +                                   +
// * |           body bytes              |
// * +                                   +
// * |            ... ...                |
// * +-----------------------------------+.
type testCodec struct {
	discardBytes int
}

func (codec testCodec) Encode(buf []byte) ([]byte, error) {
	bodyOffset := magicNumberSize + bodySize
	msgLen := bodyOffset + len(buf)

	data := make([]byte, msgLen)
	copy(data, magicNumberBytes)

	binary.BigEndian.PutUint32(data[magicNumberSize:bodyOffset], uint32(len(buf)))
	copy(data[bodyOffset:msgLen], buf)
	return data, nil
}

func (codec *testCodec) Decode(c Conn) ([]byte, error) {
	bodyOffset := magicNumberSize + bodySize
	buf, _ := c.Peek(bodyOffset)
	if len(buf) < bodyOffset {
		return nil, errIncompletePacket
	}

	if !bytes.Equal(magicNumberBytes, buf[:magicNumberSize]) {
		return nil, errors.New("invalid magic number")
	}

	bodyLen := binary.BigEndian.Uint32(buf[magicNumberSize:bodyOffset])
	msgLen := bodyOffset + int(bodyLen)
	if c.InboundBuffered() < msgLen {
		return nil, errIncompletePacket
	}
	buf, _ = c.Peek(msgLen)
	codec.discardBytes = msgLen

	return buf[bodyOffset:msgLen], nil
}

func (codec *testCodec) Discard(c Conn) {
	if codec.discardBytes <= 0 {
		return
	}
	_, _ = c.Discard(codec.discardBytes)
	codec.discardBytes = 0
}

func (codec testCodec) Unpack(buf []byte) ([]byte, error) {
	bodyOffset := magicNumberSize + bodySize
	if len(buf) < bodyOffset {
		return nil, errIncompletePacket
	}

	if !bytes.Equal(magicNumberBytes, buf[:magicNumberSize]) {
		return nil, errors.New("invalid magic number")
	}

	bodyLen := binary.BigEndian.Uint32(buf[magicNumberSize:bodyOffset])
	msgLen := bodyOffset + int(bodyLen)
	if len(buf) < msgLen {
		return nil, errIncompletePacket
	}

	return buf[bodyOffset:msgLen], nil
}

func TestSimServer(t *testing.T) {
	t.Run("packet-size=128,batch=100", func(t *testing.T) {
		testSimServer(t, ":7200", 10, 128, 100)
	})
	t.Run("packet-size=256,batch=50", func(t *testing.T) {
		testSimServer(t, ":7201", 10, 256, 50)
	})
	t.Run("packet-size=512,batch=30", func(t *testing.T) {
		testSimServer(t, ":7202", 10, 512, 30)
	})
	t.Run("packet-size=1024,batch=20", func(t *testing.T) {
		testSimServer(t, ":7203", 10, 1024, 20)
	})
	t.Run("packet-size=64*1024,batch=5", func(t *testing.T) {
		testSimServer(t, ":7204", 10, 64*1024, 5)
	})
	t.Run("packet-size=128*1024,batch=3", func(t *testing.T) {
		testSimServer(t, ":7205", 10, 128*1024, 3)
	})
	t.Run("packet-size=1024*1024,batch=1", func(t *testing.T) {
		testSimServer(t, ":7206", 10, 1024*1024, 1)
	})
}

func testSimServer(t *testing.T, addr string, nclients, packetSize, packetBatch int) {
	ts := &simServer{
		tester:      t,
		network:     "tcp",
		addr:        addr,
		multicore:   true,
		nclients:    nclients,
		packetSize:  packetSize,
		packetBatch: packetBatch,
	}
	err := Run(ts,
		ts.network+"://"+ts.addr,
		WithMulticore(ts.multicore),
		WithTicker(true),
		WithTCPKeepAlive(time.Minute*1))
	assert.NoError(t, err)
}

func runClient(t *testing.T, network, addr string, packetSize, batch int) {
	rand.Seed(time.Now().UnixNano())
	c, err := net.Dial(network, addr)
	require.NoError(t, err)
	defer c.Close()
	rd := bufio.NewReader(c)
	msg, err := rd.ReadBytes('\n')
	require.NoError(t, err)
	require.Equal(t, string(msg), "sweetness\r\n", "bad header")
	var duration time.Duration
	packetBytes := packetSize * batch
	switch {
	case packetBytes < 16*1024:
		duration = 2 * time.Second
	case packetBytes < 32*1024:
		duration = 3 * time.Second
	case packetBytes < 480*1024:
		duration = 4 * time.Second
	default:
		duration = 5 * time.Second
	}
	t.Logf("test duration: %ds", duration/time.Second)
	start := time.Now()
	for time.Since(start) < duration {
		batchWriteAndVerify(t, c, rd, packetSize, batch)
	}
}

func batchWriteAndVerify(t *testing.T, c net.Conn, rd *bufio.Reader, packetSize, batch int) {
	codec := testCodec{}
	var (
		requests  [][]byte
		buf       []byte
		packetLen int
	)
	for i := 0; i < batch; i++ {
		req := make([]byte, packetSize)
		_, err := rand.Read(req)
		require.NoError(t, err)
		requests = append(requests, req)
		packet, _ := codec.Encode(req)
		packetLen = len(packet)
		buf = append(buf, packet...)
	}
	_, err := c.Write(buf)
	require.NoError(t, err)
	respPacket := make([]byte, batch*packetLen)
	_, err = io.ReadFull(rd, respPacket)
	require.NoError(t, err)
	for i, req := range requests {
		rsp, err := codec.Unpack(respPacket[i*packetLen:])
		require.NoError(t, err)
		require.Equalf(t, req, rsp, "request and response mismatch, packet size: %d, batch: %d", packetSize, batch)
	}
}
