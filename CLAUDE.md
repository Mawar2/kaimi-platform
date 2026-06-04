# Claude Code Setup & Context

This file contains setup instructions and context specifically for Claude Code sessions working on the Kaimi project.

## GitHub MCP Server Setup

The project has a GitHub MCP (Model Context Protocol) server configured, which gives Claude Code direct access to GitHub issues, pull requests, and repository data.

### Initial Setup

If you're starting a new Claude Code session and the GitHub MCP server isn't configured, add it with:

```bash
claude mcp add --transport http github https://api.githubcopilot.com/mcp/ \
  --header "Authorization: Bearer YOUR_GITHUB_PAT"
```

Replace `YOUR_GITHUB_PAT` with a GitHub Personal Access Token that has access to the `Mawar2/Kaimi` repository.

### What This Enables

With the GitHub MCP server connected, Claude can:
- Fetch and read GitHub issues directly
- Access pull request details
- Query repository information
- Work with GitHub data without requiring the `gh` CLI

### Verifying the Connection

Run `/mcp` in your Claude Code session to verify the GitHub server is connected.

## Repository Information

- **GitHub Repository**: `Mawar2/Kaimi`
- **Main Branch**: `main`
- **Remote URL**: https://github.com/Mawar2/Kaimi.git

## Project Context

See the following files for detailed project information:
- `PROJECT.md` - Project overview and goals
- `ARCHITECTURE.md` - System architecture and design decisions
- `CONVENTIONS.md` - Coding conventions and standards
- `docs/DEVELOPER_SETUP.md` - Developer environment setup

## Working with Issues

The project uses GitHub issues for task tracking, organized by:
- **Phase labels**: `phase-0`, `phase-1`, `phase-2`, `phase-3`
- **Zone labels**: `zone-1` (Malik), `zone-2` (Timm)
- **Agent labels**: `agent:hunter`, `agent:scorer`, `agent:outline`, `agent:final-review`
- **Team labels**: `malik`, `timm`

Local ticket files are also maintained:
- `kaimi_malik_tickets.md` - Malik's ticket tracking
- `kaimi_timm_tickets.md` - Timm's ticket tracking

## Tips for Claude Sessions

- Always check the current git status and recent commits for context
- Reference issue numbers when making commits (format: `<issue#>_description`)
- Use the GitHub MCP server to fetch fresh issue data when needed
- The project is built in Go and uses Google Cloud Platform (GCP) services
