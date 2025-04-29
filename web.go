package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"syscall"
)

type Config struct {
	ServerAddr string
	Mountpoint string
	Username   string
	Password   string
	OutputFile string
}

type PageData struct {
	Config     Config
	Status     string
	IsRunning  bool
	OutputFile string
	Messages   []string
	ServerIP   string
	RTCMData   string
	Files      []string
}

var (
	clientConfig Config
	clientCmd    *exec.Cmd
	pageData     PageData
	mutex        sync.Mutex
	rtcmData     string
	rtcmBuffer   []byte // Rolling buffer for RTCM data
	autoRefresh  = true // Auto-refresh toggle
)

const RTCM_BUFFER_SIZE = 4096 // Show last 4KB of data

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "localhost"
}

func addMessage(msg string) {
	mutex.Lock()
	defer mutex.Unlock()
	pageData.Messages = append(pageData.Messages, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	if len(pageData.Messages) > 10 {
		pageData.Messages = pageData.Messages[1:]
	}
}

func updateRTCMData(data []byte) {
	mutex.Lock()
	defer mutex.Unlock()
	// Append new data to the rolling buffer
	rtcmBuffer = append(rtcmBuffer, data...)
	if len(rtcmBuffer) > RTCM_BUFFER_SIZE {
		rtcmBuffer = rtcmBuffer[len(rtcmBuffer)-RTCM_BUFFER_SIZE:]
	}
	// Format the buffer for display
	rtcmData = hex.Dump(rtcmBuffer)
	pageData.RTCMData = rtcmData
}

func getFiles() []string {
	files, err := filepath.Glob("rtcm_data.bin_*")
	if err != nil {
		return nil
	}
	return files
}

func convertToReadable(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	// Create a new file with .txt extension
	outputFile := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".txt"
	
	// Convert binary data to readable format
	var sb strings.Builder
	sb.WriteString("RTCM Data Dump\n")
	sb.WriteString("==============\n\n")
	
	// Process data in chunks
	for i := 0; i < len(data); i += 16 {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		
		// Write offset
		sb.WriteString(fmt.Sprintf("%08x  ", i))
		
		// Write hex values
		for j := i; j < end; j++ {
			sb.WriteString(fmt.Sprintf("%02x ", data[j]))
		}
		
		// Add padding if needed
		if end < i+16 {
			sb.WriteString(strings.Repeat("   ", i+16-end))
		}
		
		// Write ASCII representation
		sb.WriteString(" |")
		for j := i; j < end; j++ {
			if data[j] >= 32 && data[j] <= 126 {
				sb.WriteByte(data[j])
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	
	return ioutil.WriteFile(outputFile, []byte(sb.String()), 0644)
}

func isClientRunning() bool {
	mutex.Lock()
	defer mutex.Unlock()
	if clientCmd == nil || clientCmd.Process == nil {
		return false
	}
	// Check if process is still running (Unix)
	err := clientCmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if r.FormValue("action") == "convert" {
			filename := r.FormValue("file")
			err := convertToReadable(filename)
			if err != nil {
				addMessage(fmt.Sprintf("Error converting file: %v", err))
			} else {
				addMessage(fmt.Sprintf("Successfully converted %s to readable format", filename))
			}
		} else if r.FormValue("action") == "pause_refresh" {
			autoRefresh = !autoRefresh
		} else {
			clientConfig.ServerAddr = r.FormValue("server")
			clientConfig.Mountpoint = r.FormValue("mountpoint")
			clientConfig.Username = r.FormValue("username")
			clientConfig.Password = r.FormValue("password")
			clientConfig.OutputFile = r.FormValue("output")
			action := r.FormValue("action")
			if action == "start" {
				go startClient()
			} else if action == "stop" {
				stopClient()
			}
		}
	}

	pageData.Config = clientConfig
	pageData.OutputFile = fmt.Sprintf("%s_%s", clientConfig.OutputFile, time.Now().Format("20060102_150405"))
	pageData.Files = getFiles()
	pageData.IsRunning = isClientRunning()
	if pageData.IsRunning {
		pageData.Status = "Client running"
	} else {
		pageData.Status = "Client stopped"
	}

	tmpl := template.Must(template.New("index").Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>NTRIP Client Control</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .form-group { margin-bottom: 15px; }
        label { display: block; margin-bottom: 5px; }
        input[type="text"], input[type="password"] { width: 100%; padding: 8px; margin-bottom: 10px; }
        .button-group { margin-top: 20px; }
        button { padding: 10px 20px; margin-right: 10px; cursor: pointer; }
        button:disabled { opacity: 0.5; cursor: not-allowed; }
        .status { margin-top: 20px; padding: 10px; border: 1px solid #ccc; }
        .messages { margin-top: 20px; padding: 10px; border: 1px solid #ccc; max-height: 200px; overflow-y: auto; }
        .message { margin: 5px 0; padding: 5px; background-color: #f5f5f5; }
        .connection-info { margin-top: 20px; padding: 10px; background-color: #e6f7ff; border: 1px solid #91d5ff; }
        .data-display { margin-top: 20px; padding: 10px; border: 1px solid #ccc; background-color: #f8f8f8; font-family: monospace; white-space: pre; overflow-x: auto; max-height: 300px; overflow-y: auto; }
        .data-display pre { margin: 0; padding: 0; }
        .files-list { margin-top: 20px; padding: 10px; border: 1px solid #ccc; }
        .file-item { margin: 5px 0; padding: 5px; background-color: #f5f5f5; display: flex; justify-content: space-between; align-items: center; }
        .refresh-controls { margin-top: 10px; }
    </style>
    <script>
        var autoRefresh = {{if .IsRunning}}true{{else}}false{{end}};
        var refreshEnabled = {{if .IsRunning}}true{{else}}false{{end}};
        var userAutoRefresh = {{if .IsRunning}}true{{else}}false{{end}};
        function refreshData() {
            if (autoRefresh && refreshEnabled) {
                setTimeout(function() { location.reload(); }, 1000);
            }
        }
        window.onload = function() { refreshData(); };
    </script>
</head>
<body>
    <h1>NTRIP Client Control</h1>
    <div class="connection-info">
        <h3>Connection Information</h3>
        <p>Server IP: {{.ServerIP}}</p>
        <p>Access this interface from other devices on your network using the IP address above.</p>
    </div>
    <form method="post">
        <div class="form-group">
            <label for="server">Server Address:</label>
            <input type="text" id="server" name="server" value="{{.Config.ServerAddr}}" required>
        </div>
        <div class="form-group">
            <label for="mountpoint">Mountpoint:</label>
            <input type="text" id="mountpoint" name="mountpoint" value="{{.Config.Mountpoint}}" required>
        </div>
        <div class="form-group">
            <label for="username">Username (optional):</label>
            <input type="text" id="username" name="username" value="{{.Config.Username}}">
            <small>Leave empty if no authentication required</small>
        </div>
        <div class="form-group">
            <label for="password">Password (optional):</label>
            <input type="password" id="password" name="password" value="{{.Config.Password}}">
            <small>Leave empty if no authentication required</small>
        </div>
        <div class="form-group">
            <label for="output">Output File:</label>
            <input type="text" id="output" name="output" value="{{.Config.OutputFile}}" required>
        </div>
        <div class="button-group">
            <button type="submit" name="action" value="start" {{if .IsRunning}}disabled{{end}}>Start Client</button>
            <button type="submit" name="action" value="stop" {{if not .IsRunning}}disabled{{end}}>Stop Client</button>
        </div>
    </form>
    <div class="status">
        <h3>Status</h3>
        <p>Client Status: {{.Status}}</p>
        <p>Output File: {{.OutputFile}}</p>
    </div>
    <div class="refresh-controls">
        <form method="post" style="display:inline;">
            <button type="submit" name="action" value="pause_refresh">{{if .IsRunning}}Pause Auto-Refresh{{else}}Resume Auto-Refresh{{end}}</button>
        </form>
    </div>
    <div class="data-display">
        <h3>RTCM Data (last 4KB)</h3>
        {{if .RTCMData}}
            <pre>{{.RTCMData}}</pre>
        {{else}}
            <p>No data received yet</p>
        {{end}}
    </div>
    <div class="files-list">
        <h3>Saved Files</h3>
        {{range .Files}}
        <div class="file-item">
            <span>{{.}}</span>
            <form method="post" style="display: inline;">
                <input type="hidden" name="file" value="{{.}}">
                <button type="submit" name="action" value="convert">Convert to Text</button>
            </form>
        </div>
        {{else}}
        <p>No files saved yet</p>
        {{end}}
    </div>
    <div class="messages">
        <h3>Recent Messages</h3>
        {{range .Messages}}
        <div class="message">{{.}}</div>
        {{end}}
    </div>
</body>
</html>
`))
	tmpl.Execute(w, pageData)
}

func startClient() {
	mutex.Lock()
	if clientCmd != nil {
		addMessage("Client already running")
		mutex.Unlock()
		return
	}
	mutex.Unlock()

	// Build command
	args := []string{
		"run", "client.go",
		"-server", clientConfig.ServerAddr,
		"-mountpoint", clientConfig.Mountpoint,
		"-output", clientConfig.OutputFile,
	}

	if clientConfig.Username != "" {
		args = append(args, "-username", clientConfig.Username)
		addMessage("Using authentication with username: " + clientConfig.Username)
	}
	if clientConfig.Password != "" {
		args = append(args, "-password", clientConfig.Password)
	}

	clientCmd = exec.Command("go", args...)
	clientCmd.Stderr = os.Stderr
	
	// Create a pipe to capture output
	stdout, err := clientCmd.StdoutPipe()
	if err != nil {
		addMessage(fmt.Sprintf("Error creating pipe: %v", err))
		return
	}

	// Start client
	err = clientCmd.Start()
	if err != nil {
		addMessage(fmt.Sprintf("Error starting client: %v", err))
		return
	}

	addMessage("Client started successfully")
	addMessage(fmt.Sprintf("Connecting to %s, mountpoint: %s", clientConfig.ServerAddr, clientConfig.Mountpoint))
	
	// Start a goroutine to read the output
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				updateRTCMData(buf[:n])
			}
		}
	}()
	
	mutex.Lock()
	pageData.Status = "Client started"
	pageData.IsRunning = true
	mutex.Unlock()
}

func stopClient() {
	mutex.Lock()
	if clientCmd == nil {
		addMessage("No client running")
		mutex.Unlock()
		return
	}
	mutex.Unlock()

	err := clientCmd.Process.Kill()
	if err != nil {
		addMessage(fmt.Sprintf("Error stopping client: %v", err))
		return
	}

	addMessage("Client stopped successfully")

	mutex.Lock()
	clientCmd = nil
	pageData.Status = "Client stopped"
	pageData.IsRunning = false
	mutex.Unlock()
}

func main() {
	// Initialize default configuration
	clientConfig = Config{
		ServerAddr: "localhost:2101",
		Mountpoint: "RTCM3",
		OutputFile: "rtcm_data.bin",
	}

	pageData = PageData{
		Config:    clientConfig,
		Status:    "Not running",
		IsRunning: false,
		Messages:  make([]string, 0),
		ServerIP:  getLocalIP(),
	}

	// Start web server
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	http.HandleFunc("/", handleRoot)
	
	// Get local IP address
	localIP := getLocalIP()
	log.Printf("Starting web server on %s:%d", localIP, *port)
	log.Printf("Access the interface from other devices on your network at: http://%s:%d", localIP, *port)
	
	// Listen on all interfaces
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
} 