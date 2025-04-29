package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/tarm/serial"
	"gopkg.in/yaml.v3"
)

const (
	defaultPort = 2101 // Standard NTRIP port
)

type Config struct {
	Server struct {
		Port    int    `yaml:"port"`
		Host    string `yaml:"host"`
		Timeout int    `yaml:"timeout"`
	} `yaml:"server"`
	Serial struct {
		Port     string `yaml:"port"`
		BaudRate int    `yaml:"baud_rate"`
		DataBits int    `yaml:"data_bits"`
		StopBits int    `yaml:"stop_bits"`
		Parity   string `yaml:"parity"`
	} `yaml:"serial"`
}

type NtripServer struct {
	config   Config
	port     int
	listener net.Listener
	clients  map[net.Conn]bool
	serial   *serial.Port
}

func NewNtripServer(config Config) *NtripServer {
	return &NtripServer{
		config:  config,
		port:    config.Server.Port,
		clients: make(map[net.Conn]bool),
	}
}

func (s *NtripServer) initSerial() error {
	c := &serial.Config{
		Name:     s.config.Serial.Port,
		Baud:     s.config.Serial.BaudRate,
		Size:     byte(s.config.Serial.DataBits),
		StopBits: serial.StopBits(s.config.Serial.StopBits),
	}

	switch s.config.Serial.Parity {
	case "N":
		c.Parity = serial.ParityNone
	case "E":
		c.Parity = serial.ParityEven
	case "O":
		c.Parity = serial.ParityOdd
	default:
		return fmt.Errorf("invalid parity setting: %s", s.config.Serial.Parity)
	}

	var err error
	s.serial, err = serial.OpenPort(c)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %v", err)
	}

	log.Printf("Serial port %s opened at %d baud", s.config.Serial.Port, s.config.Serial.BaudRate)
	return nil
}

func (s *NtripServer) Start() error {
	// Initialize serial port
	if err := s.initSerial(); err != nil {
		return err
	}

	// Start TCP server
	var err error
	bindAddr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.port)
	s.listener, err = net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	log.Printf("NTRIP server started on %s", bindAddr)

	// Start reading from serial port
	go s.readSerialData()
	// Start accepting connections
	go s.acceptConnections()

	return nil
}

func (s *NtripServer) readSerialData() {
	buf := make([]byte, 1024)
	for {
		n, err := s.serial.Read(buf)
		if err != nil {
			log.Printf("Error reading from serial port: %v", err)
			continue
		}

		// Forward data to all connected clients
		data := buf[:n]
		for conn := range s.clients {
			_, err := conn.Write(data)
			if err != nil {
				log.Printf("Error writing to client: %v", err)
				conn.Close()
				delete(s.clients, conn)
			}
		}
	}
}

func (s *NtripServer) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		s.clients[conn] = true
		go s.handleClient(conn)
	}
}

func (s *NtripServer) handleClient(conn net.Conn) {
	defer func() {
		conn.Close()
		delete(s.clients, conn)
	}()

	// Send NTRIP header
	header := "ICY 200 OK\r\n"
	if _, err := conn.Write([]byte(header)); err != nil {
		log.Printf("Error sending header to client: %v", err)
		return
	}

	// Keep connection alive
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from client: %v", err)
			}
			return
		}
	}
}

func (s *NtripServer) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
	if s.serial != nil {
		s.serial.Close()
	}
	for conn := range s.clients {
		conn.Close()
	}
}

func loadConfig(path string) (Config, error) {
	var config Config
	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("error reading config file: %v", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("error parsing config file: %v", err)
	}

	return config, nil
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	server := NewNtripServer(config)
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
	server.Stop()
} 