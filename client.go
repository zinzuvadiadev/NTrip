package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

type NtripClient struct {
	serverAddr string
	mountpoint string
	username   string
	password   string
	outputFile string
}

func NewNtripClient(serverAddr, mountpoint, username, password, outputFile string) *NtripClient {
	return &NtripClient{
		serverAddr: serverAddr,
		mountpoint: mountpoint,
		username:   username,
		password:   password,
		outputFile: outputFile,
	}
}

func (c *NtripClient) Connect() error {
	// Connect to the NTRIP server
	conn, err := net.Dial("tcp", c.serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Create output file
	file, err := os.Create(c.outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Send NTRIP request
	request := fmt.Sprintf("GET /%s HTTP/1.0\r\n", c.mountpoint)
	if c.username != "" && c.password != "" {
		auth := fmt.Sprintf("%s:%s", c.username, c.password)
		request += fmt.Sprintf("Authorization: Basic %s\r\n", auth)
	}
	request += "User-Agent: NTRIP Client\r\n"
	request += "Connection: close\r\n\r\n"

	_, err = conn.Write([]byte(request))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	// Read and parse response
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	response := string(buf[:n])
	if response[:3] != "ICY" {
		return fmt.Errorf("invalid server response: %s", response)
	}

	log.Printf("Connected to NTRIP server, receiving RTCM data...")

	// Create a buffer for RTCM data
	rtcmBuffer := make([]byte, 1024)
	rtcmFile, err := os.Create(c.outputFile)
	if err != nil {
		return fmt.Errorf("failed to create RTCM file: %v", err)
	}
	defer rtcmFile.Close()

	// Read and save RTCM data
	for {
		n, err := conn.Read(rtcmBuffer)
		if err != nil {
			if err == io.EOF {
				log.Println("Connection closed by server")
				break
			}
			return fmt.Errorf("error reading RTCM data: %v", err)
		}

		// Write RTCM data to file
		_, err = rtcmFile.Write(rtcmBuffer[:n])
		if err != nil {
			return fmt.Errorf("error writing RTCM data to file: %v", err)
		}

		// Output RTCM data to stdout for web interface
		os.Stdout.Write(rtcmBuffer[:n])

		// Log the data size for monitoring
		log.Printf("Received %d bytes of RTCM data", n)
	}

	return nil
}

func main() {
	serverAddr := flag.String("server", "localhost:2101", "NTRIP server address")
	mountpoint := flag.String("mountpoint", "RTCM3", "NTRIP mountpoint")
	username := flag.String("username", "", "NTRIP username")
	password := flag.String("password", "", "NTRIP password")
	outputFile := flag.String("output", "rtcm_data.bin", "Output file for RTCM data")
	flag.Parse()

	client := NewNtripClient(*serverAddr, *mountpoint, *username, *password, *outputFile)

	// Add timestamp to output filename
	timestamp := time.Now().Format("20060102_150405")
	client.outputFile = fmt.Sprintf("%s_%s", *outputFile, timestamp)

	log.Printf("Starting NTRIP client...")
	log.Printf("Connecting to %s, mountpoint: %s", *serverAddr, *mountpoint)
	log.Printf("Output file: %s", client.outputFile)

	if err := client.Connect(); err != nil {
		log.Fatalf("Error: %v", err)
	}
} 