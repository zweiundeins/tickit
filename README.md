# Tickit

Tickit is a modern, full-stack application designed for efficient ticket management and event organization. It leverages Go for backend services and a JavaScript/TypeScript stack for the frontend, providing a robust and scalable solution.

## Features
- Ticket management and event organization
- Modular architecture with clear separation of backend and frontend
- Containerized for easy deployment (Docker)

## Technologies Used
- **Go** (backend)
- **Node.js**, **TypeScript**, **esbuild** (frontend)
- **Docker** (containerization)
- **Taskfile** (task automation)

## Getting Started
1. **Clone the repository:**
   ```bash
   git clone <repo-url>
   cd tickit
   ```
2. **Install dependencies:**
   - Backend: `go mod download`
   - Frontend: `pnpm install` (requires [pnpm](https://pnpm.io/))
3. **Build and run:**
   - Use the provided `Taskfile.yml` or Dockerfile for streamlined development and deployment.

## Common Commands

This project uses [Taskfile](https://taskfile.dev/) for automation. You can run tasks using the Go tool:

- **Build for production:**
  ```bash
  go tool task build
  ```
- **Start development server (live reload):**
  ```bash
  go tool task live
  ```

## Project Structure
- `cmd/` - Main application entry points
- `internal/` - Backend application logic
- `web/` - Frontend source code
- `data/` - Data files and assets
