// internal/config/commands/mcp_config.go
//
// MCP command configuration: metadata + options. The single source of truth
// for `vaultmind mcp`.

package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// McpMetadata defines the mcp command.
var McpMetadata = config.CommandMetadata{
	Use:   "mcp",
	Short: "Serve the vault to MCP clients (Claude Desktop, Cursor, any agent) over stdio",
	Long: "Run an MCP server on stdin/stdout, so an agent without a shell (Claude Desktop, or any " +
		"MCP client) can use the vault. Its tools are ask, search, note_get, tree, links, " +
		"note_create and import; each runs the vaultmind command of the same name with --json and " +
		"returns its output, so a tool answers exactly what the CLI answers.\n\n" +
		"The vaults are fixed when the server starts: --vault, or --vaults a,b,c to read across " +
		"several (writes, imports and links go to the first). A client cannot point a tool at " +
		"another directory.\n\n" +
		"Claude Code:\n" +
		"  claude mcp add vaultmind -- vaultmind mcp --vault /path/to/vault\n\n" +
		"Claude Desktop, Cursor (.cursor/mcp.json) and most clients take the same entry:\n" +
		"  {\"mcpServers\": {\"vaultmind\": {\"command\": \"vaultmind\",\n" +
		"    \"args\": [\"mcp\", \"--vault\", \"/path/to/vault\"]}}}\n\n" +
		"Each call starts the command afresh, as the CLI does, so ask takes about as long as " +
		"running it in a shell.",
	ConfigPrefix: "app.mcp",
	FlagOverrides: map[string]string{
		"app.mcp.vault":  "vault",
		"app.mcp.vaults": "vaults",
	},
	SeeAlso: []string{"ask", "tree", "hooks install"},
}

// McpOptions returns the mcp command's options.
func McpOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.mcp.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.mcp.vaults", DefaultValue: "", Description: "Serve several vaults, comma-separated (overrides --vault); writes go to the first", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(McpOptions)
}
