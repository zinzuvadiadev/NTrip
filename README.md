# NTRIP Caster Server

This is an NTRIP (Networked Transport of RTCM via Internet Protocol) caster server implementation in Go. It allows GPS devices to receive RTCM correction data over the internet.

## Features
- NTRIP protocol implementation
- RTCM data handling
- Basic authentication support
- Mountpoint management

## Setup
1. Install Go 1.21 or later
2. Clone this repository
3. Run `go mod init ntrip`
4. Run `go mod tidy`
5. Configure the server in `config.yaml`
6. Run `go run main.go`

## Configuration
The server can be configured using the `config.yaml` file. See the example configuration for details.

## Usage
Connect your GPS device to the server using the NTRIP client protocol. The server will handle the RTCM data distribution. # NTrip
