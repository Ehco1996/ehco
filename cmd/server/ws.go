package main

import (
	"flag"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net"
	"net/http"
	"sync"

	_ "github.com/gorilla/websocket"
)

var addr = flag.String("addr", "localhost:8080", "http service address")

var upgrader = websocket.Upgrader{} // use default options

type MyWsConn struct {
	Conn *websocket.Conn
}

func (m MyWsConn) Write(msg []byte) (n int, err error) {
	log.Println("write", msg)
	err = m.Conn.WriteMessage(websocket.BinaryMessage, msg)
	return len(msg), err
}

func (m MyWsConn) Read(b []byte) (n int, err error) {
	_, b, err = m.Conn.ReadMessage()
	log.Println("read", string(b))
	n = len(b)
	return n, err
}

func NewMyWsConn(conn *websocket.Conn) *MyWsConn {
	return &MyWsConn{Conn: conn}
}

const BUFFER_SIZE = 4 * 1024

// 全局pool
var inboundBufferPool, outboundBufferPool *sync.Pool

func init() {
	inboundBufferPool = newBufferPool(BUFFER_SIZE)
	outboundBufferPool = newBufferPool(BUFFER_SIZE)
}

func newBufferPool(size int) *sync.Pool {
	return &sync.Pool{New: func() interface{} {
		return make([]byte, size)
	}}
}

func doCopy(dst io.Writer, src io.Reader, bufferPool *sync.Pool, wg *sync.WaitGroup) {
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)
	_, err := io.CopyBuffer(dst, src, buf)
	if err != nil && err != io.EOF {
		log.Printf("failed to relay: %v\n", err)
	}
	wg.Done()
}

func relay(w http.ResponseWriter, r *http.Request) {

	c, err := upgrader.Upgrade(w, r, nil)

	wsc := NewMyWsConn(c)

	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	rc, _ := net.Dial("tcp", "0.0.0.0:9001")
	defer rc.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go doCopy(wsc, rc, inboundBufferPool, &wg)
	go doCopy(rc, wsc, outboundBufferPool, &wg)
	wg.Wait()

	//for {
	//	mt, message, err := c.ReadMessage()
	//	if err != nil {
	//		log.Println("read error:", err)
	//		break
	//	}
	//	log.Printf("recv: %s", message)
	//	rc.Write(message)
	//	log.Printf("send: %s", message)
	//
	//	buf := make([]byte, len(message))
	//	_, err := rc.Read(buf)
	//	if err != nil {
	//		log.Print(err)
	//	}
	//	log.Println(string(buf))
	//	err = c.WriteMessage(mt, buf)
	//	if err != nil {
	//		log.Println("write error:", err)
	//		break
	//	}
	//}
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	http.HandleFunc("/echo", relay)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
