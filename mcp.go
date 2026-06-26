package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"
)

// mcpClientPool holds per-server MCP clients that persist for the session lifetime.
type mcpClientPool struct {
	mu      sync.Mutex
	clients map[string]*client.Client
}

func newMCPClientPool() *mcpClientPool {
	return &mcpClientPool{clients: make(map[string]*client.Client)}
}

// get returns a cached client for the named server, creating one if necessary.
func (p *mcpClientPool) get(ctx context.Context, name string, server MCPServerConfig) (*client.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cli, ok := p.clients[name]; ok {
		return cli, nil
	}
	cli, err := initMcpClient(ctx, server)
	if err != nil {
		return nil, err
	}
	p.clients[name] = cli
	return cli, nil
}

// closeAll closes every cached client and empties the pool.
func (p *mcpClientPool) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, cli := range p.clients {
		cli.Close() //nolint:errcheck,gosec
	}
	p.clients = nil
}

func enabledMCPs(cfg *Config) iter.Seq2[string, MCPServerConfig] {
	return func(yield func(string, MCPServerConfig) bool) {
		names := slices.Collect(maps.Keys(cfg.MCPServers))
		slices.Sort(names)
		for _, name := range names {
			if !isMCPEnabled(cfg, name) {
				continue
			}
			if !yield(name, cfg.MCPServers[name]) {
				return
			}
		}
	}
}

func isMCPEnabled(cfg *Config, name string) bool {
	return !slices.Contains(cfg.MCPDisable, "*") &&
		!slices.Contains(cfg.MCPDisable, name)
}

// mcpList is a CLI top-level command; uses the process-global config directly.
func mcpList() {
	for name := range config.MCPServers {
		s := name
		if isMCPEnabled(&config, name) {
			s += stdoutStyles().Timeago.Render(" (enabled)")
		}
		fmt.Println(s)
	}
}

func mcpListTools(ctx context.Context) error {
	servers, err := mcpTools(ctx)
	if err != nil {
		return err
	}
	for sname, tools := range servers {
		for _, tool := range tools {
			fmt.Print(stdoutStyles().Timeago.Render(sname + " > "))
			fmt.Println(tool.Name)
		}
	}
	return nil
}

// mcpTools is the package-level version used by mcpListTools (--mcp-list-tools).
// Creates and closes clients transiently; does not use the session cache.
// Uses the process-global config — CLI top-level only.
func mcpTools(ctx context.Context) (map[string][]mcp.Tool, error) {
	var mu sync.Mutex
	wg, ctx := errgroup.WithContext(ctx) // cancel siblings on first failure
	result := map[string][]mcp.Tool{}
	for sname, server := range enabledMCPs(&config) {
		wg.Go(func() error {
			serverTools, err := mcpToolsFor(ctx, sname, server)
			if errors.Is(err, context.DeadlineExceeded) {
				return modsError{
					err:    fmt.Errorf("timeout while listing tools for %q - make sure the configuration is correct. If your server requires a docker container, make sure it's running", sname),
					reason: "Could not list tools",
				}
			}
			if err != nil {
				return modsError{
					err:    err,
					reason: "Could not list tools",
				}
			}
			mu.Lock()
			result[sname] = append(result[sname], serverTools...)
			mu.Unlock()
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return nil, err //nolint:wrapcheck
	}
	return result, nil
}

// mcpTools is the session-scoped version used during completions.
// Uses the pool to cache clients across tool call rounds.
func (m *Mods) mcpTools(ctx context.Context) (map[string][]mcp.Tool, error) {
	var mu sync.Mutex
	wg, ctx := errgroup.WithContext(ctx)
	result := map[string][]mcp.Tool{}
	for sname, server := range enabledMCPs(m.Config) {
		wg.Go(func() error {
			cli, err := m.mcpPool.get(ctx, sname, server)
			if errors.Is(err, context.DeadlineExceeded) {
				return modsError{
					err:    fmt.Errorf("timeout while listing tools for %q - make sure the configuration is correct. If your server requires a docker container, make sure it's running", sname),
					reason: "Could not list tools",
				}
			}
			if err != nil {
				return modsError{err: err, reason: "Could not list tools"}
			}
			tools, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
			if err != nil {
				return modsError{
					err:    fmt.Errorf("could not list tools for %s: %w", sname, err),
					reason: "Could not list tools",
				}
			}
			mu.Lock()
			result[sname] = append(result[sname], tools.Tools...)
			mu.Unlock()
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return nil, err //nolint:wrapcheck
	}
	return result, nil
}

// initMcpClient creates and initializes an MCP client.
func initMcpClient(ctx context.Context, server MCPServerConfig) (*client.Client, error) {
	var cli *client.Client
	var err error

	switch server.Type {
	case "", "stdio":
		cli, err = client.NewStdioMCPClient(
			server.Command,
			append(os.Environ(), server.Env...),
			server.Args...,
		)
	case "sse":
		cli, err = client.NewSSEMCPClient(server.URL)
	case "http":
		cli, err = client.NewStreamableHttpClient(server.URL)
	default:
		return nil, fmt.Errorf("unsupported MCP server type: %q, supported types are: stdio, sse, http", server.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	if err := cli.Start(ctx); err != nil {
		cli.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}

	if _, err := cli.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		cli.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return cli, nil
}

// mcpToolsFor is used by the package-level mcpTools (transient, no cache).
func mcpToolsFor(ctx context.Context, name string, server MCPServerConfig) ([]mcp.Tool, error) {
	cli, err := initMcpClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("could not setup %s: %w", name, err)
	}
	defer cli.Close() //nolint:errcheck

	tools, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("could not setup %s: %w", name, err)
	}
	return tools.Tools, nil
}

// toolCall is the session-scoped version: reuses cached clients from the pool.
func (m *Mods) toolCall(ctx context.Context, name string, data []byte) (string, error) {
	sname, tool, ok := strings.Cut(name, "_")
	if !ok {
		return "", fmt.Errorf("mcp: invalid tool name: %q", name)
	}
	server, ok := m.Config.MCPServers[sname]
	if !ok {
		return "", fmt.Errorf("mcp: invalid server name: %q", sname)
	}
	if !isMCPEnabled(m.Config, sname) {
		return "", fmt.Errorf("mcp: server is disabled: %q", sname)
	}
	cli, err := m.mcpPool.get(ctx, sname, server)
	if err != nil {
		return "", fmt.Errorf("mcp: %w", err)
	}

	var args map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &args); err != nil {
			return "", fmt.Errorf("mcp: %w: %s", err, string(data))
		}
	}

	request := mcp.CallToolRequest{}
	request.Params.Name = tool
	request.Params.Arguments = args
	result, err := cli.CallTool(ctx, request)
	if err != nil {
		return "", fmt.Errorf("mcp: %w", err)
	}

	var sb strings.Builder
	for _, content := range result.Content {
		switch content := content.(type) {
		case mcp.TextContent:
			sb.WriteString(content.Text)
		default:
			sb.WriteString("[Non-text content]")
		}
	}

	if result.IsError {
		return "", errors.New(sb.String())
	}
	return sb.String(), nil
}
