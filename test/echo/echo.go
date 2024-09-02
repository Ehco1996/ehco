//nolint:errcheck
package echo

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

type EchoServer struct {
	host        string
	port        int
	tcpListener net.Listener
	udpConn     *net.UDPConn
	stopChan    chan struct{}
	wg          sync.WaitGroup
	logger      *zap.SugaredLogger
}

func NewEchoServer(host string, port int) *EchoServer {
	return &EchoServer{
		host:     host,
		port:     port,
		stopChan: make(chan struct{}),
		logger:   zap.S().Named("echo-test-server"),
	}
}

func (s *EchoServer) Run() error {
	addr := s.host + ":" + strconv.Itoa(s.port)
	var err error

	// Start TCP server
	s.tcpListener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server: %w", err)
	}

	// Start UDP server
	udpAddr := net.UDPAddr{IP: net.ParseIP(s.host), Port: s.port}
	s.udpConn, err = net.ListenUDP("udp", &udpAddr)
	if err != nil {
		return fmt.Errorf("failed to start UDP server: %w", err)
	}

	s.logger.Infof("Echo server started at: %s", addr)

	s.wg.Add(2)
	go s.serveTCP()
	go s.serveUDP()

	return nil
}

func (s *EchoServer) Stop() {
	close(s.stopChan)
	if s.tcpListener != nil {
		s.tcpListener.Close()
	}
	if s.udpConn != nil {
		s.udpConn.Close()
	}
	s.wg.Wait()
	s.logger.Info("Echo server stopped")
}

func (s *EchoServer) serveTCP() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopChan:
			return
		default:
			conn, err := s.tcpListener.Accept()
			if err != nil {
				select {
				case <-s.stopChan:
					return
				default:
					s.logger.Errorf("Failed to accept TCP connection: %v", err)
				}
				continue
			}
			go s.handleTCPConn(conn)
		}
	}
}

func (s *EchoServer) handleTCPConn(conn net.Conn) {
	defer conn.Close()
	s.logger.Infof("New TCP connection from: %s", conn.RemoteAddr())

	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err == io.EOF {
			s.logger.Infof("Connection closed by client: %s", conn.RemoteAddr())
			return
		}
		if err != nil {
			s.logger.Errorf("Error reading from connection: %v", err)
			return
		}

		s.logger.Infof("Received from %s: %s", conn.RemoteAddr(), string(buf[:n]))

		_, err = conn.Write(buf[:n])
		if err != nil {
			s.logger.Errorf("Error writing to connection: %v", err)
			return
		}
	}
}

func isClosedConnError(err error) bool {
	return errors.Is(err, net.ErrClosed)
}

func (s *EchoServer) serveUDP() {
	defer s.wg.Done()
	buf := make([]byte, 1024)
	for {
		select {
		case <-s.stopChan:
			return
		default:
			n, remoteAddr, err := s.udpConn.ReadFromUDP(buf)
			if err != nil {
				if isClosedConnError(err) {
					break
				}
				s.logger.Errorf("Error reading UDP: %v", err)
				continue
			}

			s.logger.Infof("Received UDP from %s: %s", remoteAddr, string(buf[:n]))

			_, err = s.udpConn.WriteToUDP(buf[:n], remoteAddr)
			if err != nil {
				s.logger.Errorf("Error writing UDP: %v", err)
			}
		}
	}
}

func SendTcpMsg(msg []byte, address string) []byte {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	println("conn start", conn.RemoteAddr().String(), conn.LocalAddr().String())
	if _, err := conn.Write(msg); err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Second * 1)
	buf := make([]byte, len(msg))
	n, err := conn.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	conn.Close()
	println("conn closed", conn.RemoteAddr().String())
	return buf[:n]
}

func EchoTcpMsgLong(msg []byte, sleepTime time.Duration, address string) error {
	logger := zap.S()
	buf := make([]byte, len(msg))
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	logger.Infof("conn start %s %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
	for i := 0; i < 10; i++ {
		if _, err := conn.Write(msg); err != nil {
			return err
		}
		n, err := conn.Read(buf)
		if err != nil {
			return err
		}
		if string(buf[:n]) != string(msg) {
			return fmt.Errorf("msg not equal at %d send:%s receive:%s n:%d", i, msg, buf[:n], n)
		}
		// to fake a long connection
		time.Sleep(sleepTime)
	}
	logger.Infof("conn closed %s %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
	return nil
}

func SendUdpMsg(msg []byte, address string) []byte {
	conn, err := net.Dial("udp", address)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(msg); err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, len(msg))
	time.Sleep(time.Second * 1)
	n, _ := conn.Read(buf)
	return buf[:n]
}
