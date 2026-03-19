package builtins

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ══════════════════════════════════════════════════════════════════
// net_ping  -  ICMP reachability test
// ══════════════════════════════════════════════════════════════════

type NetPingTool struct{}

func (t *NetPingTool) Name() string { return "net_ping" }
func (t *NetPingTool) Description() string {
	return "Ping a host to check ICMP reachability, latency, and packet loss. Args: host (IP or hostname), count (optional, default 4)."
}
func (t *NetPingTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host":  map[string]interface{}{"type": "string", "description": "IP address or hostname to ping"},
			"count": map[string]interface{}{"type": "number", "description": "Number of pings (default 4, max 20)"},
		},
		"required": []string{"host"},
	}
}

func (t *NetPingTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	host := argStr(args, "host")
	if host == "" {
		return "", fmt.Errorf("missing required argument: host")
	}

	count := int(argFloat(args, "count"))
	if count <= 0 {
		count = 4
	}
	if count > 20 {
		count = 20
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", fmt.Sprintf("%d", count), host)
	} else {
		cmd = exec.CommandContext(ctx, "ping", "-c", fmt.Sprintf("%d", count), host)
	}

	out, err := runWithTimeout(cmd, 30*time.Second)
	if err != nil {
		return fmt.Sprintf("Ping to %s failed: %v\n%s", host, err, out), nil
	}
	return out, nil
}

// ══════════════════════════════════════════════════════════════════
// net_traceroute  -  hop-by-hop path discovery
// ══════════════════════════════════════════════════════════════════

type NetTracerouteTool struct{}

func (t *NetTracerouteTool) Name() string { return "net_traceroute" }
func (t *NetTracerouteTool) Description() string {
	return "Trace the network path to a destination, showing each hop with latency. Args: host (IP or hostname), max_hops (optional, default 30)."
}
func (t *NetTracerouteTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host":     map[string]interface{}{"type": "string", "description": "Destination IP or hostname"},
			"max_hops": map[string]interface{}{"type": "number", "description": "Maximum hops (default 30, max 64)"},
		},
		"required": []string{"host"},
	}
}

func (t *NetTracerouteTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	host := argStr(args, "host")
	if host == "" {
		return "", fmt.Errorf("missing required argument: host")
	}

	maxHops := int(argFloat(args, "max_hops"))
	if maxHops <= 0 {
		maxHops = 30
	}
	if maxHops > 64 {
		maxHops = 64
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "tracert", "-h", fmt.Sprintf("%d", maxHops), host)
	} else {
		cmd = exec.CommandContext(ctx, "traceroute", "-m", fmt.Sprintf("%d", maxHops), host)
	}

	out, err := runWithTimeout(cmd, 120*time.Second)
	if err != nil {
		return fmt.Sprintf("Traceroute to %s failed: %v\n%s", host, err, out), nil
	}
	return out, nil
}

// ══════════════════════════════════════════════════════════════════
// net_dns  -  DNS lookup
// ══════════════════════════════════════════════════════════════════

type NetDNSTool struct{}

func (t *NetDNSTool) Name() string { return "net_dns" }
func (t *NetDNSTool) Description() string {
	return "DNS lookup for a hostname. Returns A, AAAA, MX, NS, CNAME, or PTR records depending on type. Args: host (domain name), type (optional: A, AAAA, MX, NS, CNAME, PTR, ANY; default A), server (optional DNS server)."
}
func (t *NetDNSTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host":   map[string]interface{}{"type": "string", "description": "Domain name to resolve"},
			"type":   map[string]interface{}{"type": "string", "description": "Record type: A, AAAA, MX, NS, CNAME, PTR, ANY (default: A)"},
			"server": map[string]interface{}{"type": "string", "description": "DNS server to query (optional, e.g. 8.8.8.8)"},
		},
		"required": []string{"host"},
	}
}

func (t *NetDNSTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	host := argStr(args, "host")
	if host == "" {
		return "", fmt.Errorf("missing required argument: host")
	}

	recordType := argStr(args, "type")
	if recordType == "" {
		recordType = "A"
	}
	recordType = strings.ToUpper(recordType)

	server := argStr(args, "server")

	// Use nslookup (available on all platforms).
	cmdArgs := []string{"-type=" + recordType, host}
	if server != "" {
		cmdArgs = append(cmdArgs, server)
	}

	cmd := exec.CommandContext(ctx, "nslookup", cmdArgs...)
	out, err := runWithTimeout(cmd, 15*time.Second)
	if err != nil {
		return fmt.Sprintf("DNS lookup for %s failed: %v\n%s", host, err, out), nil
	}
	return out, nil
}

// ══════════════════════════════════════════════════════════════════
// net_portscan  -  TCP port connectivity check
// ══════════════════════════════════════════════════════════════════

type NetPortScanTool struct{}

func (t *NetPortScanTool) Name() string { return "net_portscan" }
func (t *NetPortScanTool) Description() string {
	return "Check if specific TCP ports are open on a host. Args: host (IP or hostname), ports (comma-separated, e.g. '22,80,443,3389')."
}
func (t *NetPortScanTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host":  map[string]interface{}{"type": "string", "description": "Target IP or hostname"},
			"ports": map[string]interface{}{"type": "string", "description": "Comma-separated ports to check (e.g. '22,80,443')"},
		},
		"required": []string{"host", "ports"},
	}
}

func (t *NetPortScanTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	host := argStr(args, "host")
	ports := argStr(args, "ports")
	if host == "" || ports == "" {
		return "", fmt.Errorf("missing required arguments: host and ports")
	}

	portList := strings.Split(ports, ",")
	if len(portList) > 20 {
		portList = portList[:20] // safety limit
	}

	var results strings.Builder
	results.WriteString(fmt.Sprintf("Port scan results for %s:\n", host))

	for _, port := range portList {
		port = strings.TrimSpace(port)
		if port == "" {
			continue
		}

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			// PowerShell Test-NetConnection is slow, use direct TCP with .NET.
			psCmd := fmt.Sprintf(
				"$t = New-Object System.Net.Sockets.TcpClient; try { $t.ConnectAsync('%s', %s).Wait(3000); if ($t.Connected) { 'OPEN' } else { 'CLOSED' } } catch { 'CLOSED' } finally { $t.Dispose() }",
				host, port)
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", psCmd)
		} else {
			cmd = exec.CommandContext(ctx, "bash", "-c",
				fmt.Sprintf("timeout 3 bash -c 'echo >/dev/tcp/%s/%s' 2>/dev/null && echo OPEN || echo CLOSED", host, port))
		}

		out, _ := runWithTimeout(cmd, 5*time.Second)
		status := "CLOSED"
		if strings.Contains(strings.TrimSpace(out), "OPEN") {
			status = "OPEN"
		}
		results.WriteString(fmt.Sprintf("  Port %s: %s\n", port, status))
	}

	return results.String(), nil
}

// ══════════════════════════════════════════════════════════════════
// net_interfaces  -  local network interface info
// ══════════════════════════════════════════════════════════════════

type NetInterfacesTool struct{}

func (t *NetInterfacesTool) Name() string { return "net_interfaces" }
func (t *NetInterfacesTool) Description() string {
	return "Show local network interfaces with IP addresses, MAC addresses, and status."
}
func (t *NetInterfacesTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *NetInterfacesTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Get adapter info in a clean format.
		psCmd := `Get-NetAdapter | Where-Object { $_.Status -eq 'Up' } | ForEach-Object {
			$ip = (Get-NetIPAddress -InterfaceIndex $_.ifIndex -ErrorAction SilentlyContinue | Where-Object { $_.AddressFamily -eq 'IPv4' }).IPAddress
			$gw = (Get-NetRoute -InterfaceIndex $_.ifIndex -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue).NextHop
			"$($_.Name) | $($_.InterfaceDescription)"
			"  Status: $($_.Status)  Speed: $($_.LinkSpeed)  MAC: $($_.MacAddress)"
			"  IPv4: $ip  Gateway: $gw"
			""
		}`
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", psCmd)
	} else {
		cmd = exec.CommandContext(ctx, "ip", "addr", "show")
	}

	out, err := runWithTimeout(cmd, 15*time.Second)
	if err != nil {
		return fmt.Sprintf("Failed to get interfaces: %v\n%s", err, out), nil
	}
	return out, nil
}

// ══════════════════════════════════════════════════════════════════
// net_routes  -  routing table
// ══════════════════════════════════════════════════════════════════

type NetRoutesTool struct{}

func (t *NetRoutesTool) Name() string { return "net_routes" }
func (t *NetRoutesTool) Description() string {
	return "Show the local routing table including destination networks, gateways, and interface metrics."
}
func (t *NetRoutesTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *NetRoutesTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "route", "print")
	} else {
		cmd = exec.CommandContext(ctx, "ip", "route", "show")
	}

	out, err := runWithTimeout(cmd, 10*time.Second)
	if err != nil {
		return fmt.Sprintf("Failed to get routes: %v\n%s", err, out), nil
	}
	return out, nil
}

// ══════════════════════════════════════════════════════════════════
// net_arp  -  ARP table (MAC-to-IP)
// ══════════════════════════════════════════════════════════════════

type NetARPTool struct{}

func (t *NetARPTool) Name() string { return "net_arp" }
func (t *NetARPTool) Description() string {
	return "Show the ARP table mapping IP addresses to MAC addresses on the local network."
}
func (t *NetARPTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *NetARPTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	cmd := exec.CommandContext(ctx, "arp", "-a")
	out, err := runWithTimeout(cmd, 10*time.Second)
	if err != nil {
		return fmt.Sprintf("Failed to get ARP table: %v\n%s", err, out), nil
	}
	return out, nil
}

// ══════════════════════════════════════════════════════════════════
// net_ssh  -  SSH to remote device and run command
// ══════════════════════════════════════════════════════════════════

type NetSSHTool struct{}

func (t *NetSSHTool) Name() string { return "net_ssh" }
func (t *NetSSHTool) Description() string {
	return "SSH to a remote device and execute a command. Useful for network device management (routers, switches, servers). Args: host (IP or hostname), user (SSH username), command (command to run), port (optional, default 22)."
}
func (t *NetSSHTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"host":    map[string]interface{}{"type": "string", "description": "SSH host (IP or hostname)"},
			"user":    map[string]interface{}{"type": "string", "description": "SSH username"},
			"command": map[string]interface{}{"type": "string", "description": "Command to execute on remote device"},
			"port":    map[string]interface{}{"type": "number", "description": "SSH port (default 22)"},
		},
		"required": []string{"host", "user", "command"},
	}
}

func (t *NetSSHTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	host := argStr(args, "host")
	user := argStr(args, "user")
	command := argStr(args, "command")
	if host == "" || user == "" || command == "" {
		return "", fmt.Errorf("missing required arguments: host, user, command")
	}

	port := int(argFloat(args, "port"))
	if port <= 0 {
		port = 22
	}

	// Safety: block destructive commands on network devices.
	lower := strings.ToLower(command)
	for _, blocked := range []string{"write erase", "erase startup", "reload", "format", "delete /force"} {
		if strings.Contains(lower, blocked) {
			return "", fmt.Errorf("blocked: destructive command %q is not allowed via net_ssh", command)
		}
	}

	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf("%s@%s", user, host),
		command,
	}

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	out, err := runWithTimeout(cmd, 30*time.Second)
	if err != nil {
		return fmt.Sprintf("SSH to %s@%s:%d failed: %v\n%s", user, host, port, err, out), nil
	}
	return fmt.Sprintf("SSH %s@%s:%d > %s\n\n%s", user, host, port, command, out), nil
}

// ══════════════════════════════════════════════════════════════════
// Helper: run command with timeout
// ══════════════════════════════════════════════════════════════════

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	// Create a context with timeout if the command doesn't already have one.
	if cmd.ProcessState == nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// Replace command context.
		newCmd := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
		newCmd.Dir = cmd.Dir
		newCmd.Env = cmd.Env
		cmd = newCmd
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if errOut := stderr.String(); errOut != "" && output == "" {
		output = errOut
	}

	return output, err
}
