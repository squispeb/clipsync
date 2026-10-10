package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type prompter struct {
	in          *bufio.Reader
	interactive bool
	yes         bool
}

func (p *prompter) ask(question string) string {
	fmt.Print(question)
	line, _ := p.in.ReadString('\n')
	return strings.TrimSpace(line)
}

// confirm returns def without asking when --yes is set or stdin isn't a
// terminal.
func (p *prompter) confirm(question string, def bool) bool {
	if p.yes || !p.interactive {
		return def
	}
	hint := "[Y/n]"
	if !def {
		hint = "[y/N]"
	}
	switch strings.ToLower(p.ask(question + " " + hint + " ")) {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func defaultDeviceName() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(h, ".local"))
}

func cmdSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	configPath := fs.String("config", "", "config file path")
	newGroup := fs.Bool("new-group", false, "start a new group (this is the first machine)")
	join := fs.String("join", "", "join the group of this machine (Tailscale IP or name); asks for the code from `clipsync pair` there, or uses CLIPSYNC_TOKEN")
	code := fs.String("code", "", "pairing code shown by `clipsync pair` on the machine you join")
	yes := fs.Bool("yes", false, "don't ask; accept the defaults (for scripts and agents)")
	noService := fs.Bool("no-service", false, "don't install the background service")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: clipsync setup [options]")
		fmt.Fprintln(os.Stderr, "\nInteractive by default. For scripts and agents:")
		fmt.Fprintln(os.Stderr, "  clipsync setup --new-group --yes                         first machine")
		fmt.Fprintln(os.Stderr, "  clipsync setup --join <machine> --code <code> --yes      others (code from `clipsync pair` there)")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if *newGroup && *join != "" {
		return fmt.Errorf("use either --new-group or --join, not both")
	}

	p := &prompter{in: bufio.NewReader(os.Stdin), interactive: stdinIsTerminal(), yes: *yes}
	path := *configPath
	if path == "" {
		path = ConfigPath()
	}

	fmt.Println("ClipSync setup")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  1. check that Tailscale and this machine's clipboard are usable")
	fmt.Println("  2. start a new group, or join one by pairing with a machine already in it")
	fmt.Println("  3. run clipsync in the background at login (asks first)")
	fmt.Println("Clipboard contents only go to machines in your group, over Tailscale.")
	fmt.Println()

	// 1. Checks
	fmt.Println("[1/4] Checking this machine")
	selfIP, ok := TailscaleIP()
	if !ok {
		return fmt.Errorf("Tailscale isn't connected. Install it (https://tailscale.com/download), log in, and run setup again")
	}
	fmt.Printf("  ✓ Tailscale: %s\n", selfIP)
	desc, boardOK, hint := boardRequirement()
	if !boardOK {
		return fmt.Errorf("clipboard: %s is not usable: %s", desc, hint)
	}
	fmt.Printf("  ✓ Clipboard: %s\n", desc)

	cfg := DefaultConfig()
	if _, err := os.Stat(path); err == nil {
		if cfg, err = LoadConfig(path); err != nil {
			return err
		}
	}
	if cfg.DeviceName == "" {
		cfg.DeviceName = defaultDeviceName()
	}
	self := net.JoinHostPort(selfIP, strconv.Itoa(cfg.Port))

	// 2. Group
	fmt.Println()
	fmt.Println("[2/4] Group")
	keep := false
	if cfg.Token != "" && len(cfg.Peers) > 0 && !*newGroup && *join == "" {
		fmt.Printf("  This machine is already in a group with %s.\n", strings.Join(cfg.Peers, ", "))
		keep = p.confirm("  Keep it?", true)
	}
	switch {
	case keep:
	case *join != "":
		if err := joinGroup(p, &cfg, *join, *code, self); err != nil {
			return err
		}
	case *newGroup:
		if err := startGroup(&cfg); err != nil {
			return err
		}
	case p.interactive && !p.yes:
		member, err := chooseGroup(p, cfg.Port)
		if err != nil {
			return err
		}
		if member == "" {
			err = startGroup(&cfg)
		} else {
			err = joinGroup(p, &cfg, member, *code, self)
		}
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("not running interactively: pass --new-group (first machine) or --join <machine> --code <code>")
	}
	cfg.AutoDiscover = false
	if err := SaveConfig(path, cfg); err != nil {
		return err
	}
	fmt.Printf("  Saved %s\n", path)

	// 3. Service
	fmt.Println()
	fmt.Println("[3/4] Background service")
	exe, err := serviceExecutable()
	if err != nil {
		return err
	}
	installed := false
	if *noService {
		fmt.Println("  Skipped (--no-service). Run `clipsync daemon` yourself, or `clipsync service install` later.")
	} else {
		fmt.Print("  " + servicePlan(exe))
		if p.confirm("  Start clipsync now and at every login?", true) {
			if err := installService(exe); err != nil {
				return err
			}
			installed = true
			fmt.Println("  ✓ Installed")
		} else if _, running := serviceStatus(); running {
			installed = true
			fmt.Println("  Left the existing service as it is.")
		} else {
			fmt.Println("  Skipped. Run `clipsync service install` when you're ready.")
		}
	}

	// 4. Verify
	fmt.Println()
	fmt.Println("[4/4] Checking the connection")
	if installed {
		if waitForDaemon(self, cfg.Token, 15*time.Second) {
			fmt.Printf("  ✓ clipsync is running here (%s)\n", self)
		} else {
			fmt.Printf("  ✗ clipsync isn't answering at %s yet; check `clipsync service status`\n", self)
		}
	}
	for _, peer := range cfg.Peers {
		h, err := peerHealth(peer, cfg.Token)
		switch {
		case err != nil:
			fmt.Printf("  ✗ %s unreachable (%v)\n", peer, err)
		case !h.Authorized:
			fmt.Printf("  ✗ %s has a different token\n", peer)
		default:
			fmt.Printf("  ✓ %s (%s, v%s)\n", h.Device, peer, h.Version)
		}
	}

	fmt.Println()
	if len(cfg.Peers) == 0 {
		fmt.Println("Done. This is the first machine in the group.")
	} else {
		fmt.Println("Done. Copy something here and paste it on another machine.")
	}
	fmt.Println("To add a machine: run `clipsync pair` here, then `clipsync setup` on the new machine.")
	return nil
}

func startGroup(cfg *Config) error {
	t, err := newToken()
	if err != nil {
		return err
	}
	cfg.Token = t
	cfg.Peers = []string{}
	fmt.Println("  Started a new group (generated a new shared token).")
	return nil
}

func joinGroup(p *prompter, cfg *Config, member, code, self string) error {
	addr, err := normalizePeer(member, cfg.Port)
	if err != nil {
		return err
	}
	var info groupInfo
	if token := os.Getenv("CLIPSYNC_TOKEN"); token != "" {
		if err := callJSON(http.MethodGet, "http://"+addr+"/peers", token, nil, &info); err != nil {
			return fmt.Errorf("join %s with CLIPSYNC_TOKEN: %w", addr, err)
		}
		info.Token = token
	} else {
		if code == "" {
			if !p.interactive {
				return fmt.Errorf("no pairing code: run `clipsync pair` on %s and pass --code", member)
			}
			fmt.Printf("  On %s, run:  clipsync pair\n", member)
			code = p.ask("  Then enter the code it shows: ")
		}
		code = strings.ReplaceAll(strings.ReplaceAll(code, " ", ""), "-", "")
		req := pairJoinRequest{Code: code, Device: cfg.DeviceName, Port: cfg.Port}
		if err := callJSON(http.MethodPost, "http://"+addr+"/pair/join", "", req, &info); err != nil {
			return fmt.Errorf("pairing with %s failed: %w", addr, err)
		}
	}
	cfg.Token = info.Token
	cfg.Peers = []string{}
	for _, peer := range append([]string{info.Self}, info.Peers...) {
		if peer != "" && peer != self {
			cfg.Peers = MergePeerLists(cfg.Peers, []string{peer})
		}
	}
	fmt.Printf("  Joined %s's group (%d other machine(s)).\n", info.Device, len(cfg.Peers))
	return nil
}

type joinable struct {
	name, addr, version string
}

// chooseGroup lists tailnet machines running clipsync and asks which group to
// join. It returns "" to start a new group.
func chooseGroup(p *prompter, port int) (string, error) {
	fmt.Println("  Looking for clipsync on your other Tailscale machines...")
	found := findJoinable(port)
	fmt.Println()
	for i, j := range found {
		fmt.Printf("  %d) join %s (%s, v%s)\n", i+1, j.name, j.addr, j.version)
	}
	fmt.Println("  n) start a new group (this is the first machine)")
	fmt.Println("  a) enter another machine's address")
	for {
		choice := strings.ToLower(p.ask("  Choose: "))
		switch choice {
		case "n":
			return "", nil
		case "a":
			if addr := p.ask("  Tailscale IP or name: "); addr != "" {
				return addr, nil
			}
			continue
		}
		if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(found) {
			return found[n-1].addr, nil
		}
	}
}

// tailscaleCLI finds the tailscale command, including the one inside the
// macOS app, which isn't on PATH by default.
func tailscaleCLI() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	const macApp = "/Applications/Tailscale.app/Contents/MacOS/Tailscale"
	if _, err := os.Stat(macApp); err == nil {
		return macApp
	}
	return ""
}

func findJoinable(port int) []joinable {
	cli := tailscaleCLI()
	if cli == "" {
		return nil
	}
	out, err := exec.Command(cli, "status", "--json").Output()
	if err != nil {
		return nil
	}
	var st tailscaleStatus
	if json.Unmarshal(out, &st) != nil {
		return nil
	}
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		found []joinable
	)
	for _, peer := range st.Peer {
		if !peer.Online {
			continue
		}
		for _, ip := range peer.TailscaleIPs {
			if net.ParseIP(ip).To4() == nil {
				continue
			}
			wg.Add(1)
			go func(name, addr string) {
				defer wg.Done()
				if h, err := peerHealth(addr, ""); err == nil && h.Service == "clipsync" {
					mu.Lock()
					found = append(found, joinable{name: name, addr: addr, version: h.Version})
					mu.Unlock()
				}
			}(peer.HostName, net.JoinHostPort(ip, strconv.Itoa(port)))
		}
	}
	wg.Wait()
	sort.Slice(found, func(i, j int) bool { return found[i].name < found[j].name })
	return found
}

func peerHealth(addr, token string) (healthInfo, error) {
	var h healthInfo
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/health", nil)
	if err != nil {
		return h, err
	}
	if token != "" {
		req.Header.Set("X-ClipSync-Token", token)
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return h, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return h, nil // pre-0.5 daemon with a different token
	}
	if resp.StatusCode != http.StatusOK {
		return h, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&h)
	return h, err
}

func waitForDaemon(addr, token string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h, err := peerHealth(addr, token); err == nil && h.Authorized {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
