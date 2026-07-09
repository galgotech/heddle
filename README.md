# Heddle

Heddle is a hybrid programming language **transpiled to Go**, designed for logical flow declaration, application orchestration, alternative execution routing, and robust error handling in a simple, expressive, and performance manner.

The language adopts a design that clearly separates intent from implementation into two complementary paradigms:
- **"What to do" (Functional Language):** Written in declarative files with the `.he` extension (e.g., `main.he`), describing the logical flow and application orchestration using pipe operators (`|`).
- **"How to do" (Imperative Language):** Concrete implementation of complex business rules and imperative behaviors written in pure **Go**, ensuring static type safety and performance.

> [!NOTE]
> This is Heddle **V3** (the third round of improvements).\
> This project is currently in unstable development.

---

## Features

- **Hybrid Paradigm**: Clear separation between functional/declarative orchestration logic (`.he`) and imperative implementation.
- **Safe Transpilation**: Declarative files (`.he`) are analyzed and compiled/transpiled into static, strongly-typed Go code.
- **Columnar Data Model**: Internal processing and data passing utilize **Frames** organized by columns, optimizing bulk operations and enabling efficient transitions to Go structs without memory duplication.
- **Immutable Memory Management**: The Heddle flow is designed to be immutable, allocating new memory regions during transitions efficiently to avoid garbage collection overhead.
- **Handler-based Error Treatment**: Errors in individual rows of a frame are isolated and routed to specific error handlers (such as Dead Letter Queues or logging routines), keeping the rest of the application running.
- **Language Server (LSP)**: Full editor support offering real-time diagnostics, type propagation in signatures and variables, document highlighting, and detailed hover documentation.

---

## Installation

### Prerequisites
- **Go**: Version `1.26` or higher.

### Compilation
To build the `heddle` CLI tool from source:

```bash
# Build using the Makefile
make build

# Or build directly via Go
go build -o bin/heddle cmd/heddle/main.go
```
The compiled binary will be saved in the `bin/` directory.

---

## Usage

The Heddle CLI provides essential commands to verify syntax and semantics, run programs in the virtual machine, or start the LSP server.

### 1. Semantic Check
Run syntax and semantic analysis on a Heddle program without executing it:
```bash
./bin/heddle check my_program.he
```

### 2. Execution
Interpret and run a Heddle program using the runtime's virtual machine:
```bash
./bin/heddle run my_program.he
```

### 3. Language Server (LSP)
Start the language server for editor integration (VS Code, Neovim, etc.) via stdio:
```bash
./bin/heddle lsp
```

### Quick Example

Below is a simple declarative flow (`main.he`) that imports native Go packages, starts an HTTP server with a `/healthz` endpoint, and executes some logical pipelines:

```heddle
import "math"
import "strings"
import "io"
import "net/http"

http_server = http.server {
  host: "localhost",
  port: 8080
}

flow main {
  // Expose a HTTP GET endpoint for event routing
  http_server.get(path: "/healthz") {
    ok(request) {
      request.body | io.print()
      return { status: 200, body: "OK" }
    }
  }

  // Declarative data passing via pipes
  "hello from heddle!" | io.print()
  
  // String transformation and subsequent printing
  "heddle language" | strings.to_upper() | io.print()
  
  // Mathematical rounding
  15.89 | math.round() | io.print()
}
```


---

## Logical Flow Control & Error Handling

Heddle provides advanced control flow mechanisms that decouple execution paths from implementation specifics.

### 1. Alternative Route Pattern Matching
Branching logical paths can be declared dynamically using pattern matching, where alternative execution routes are controlled directly by the return variants of Go functions:

```heddle
result = "a" | cmp.compare("b") {
  less(val) {
    "Matched less!" | io.print()
    return val
  }
  equals(val) {
    "Matched equals!" | io.print()
    return val
  }
  greater(val) {
    "Matched greater!" | io.print()
    return val
  }
}
```

### 2. Declarative Error Handling
Errors returned from imperative functions are isolated and handled via specific `handler` constructs. If an operation yields an error, the flow can gracefully catch and redirect it using the `? handler_name()` operator:

```heddle
// Declare an isolated error handler
handler log_error (err) {
  err | io.print()
}

flow main {
  // If json.unmarshal fails, the error routes to the log_error handler
  user_frame = json_string | json.unmarshal() ? log_error()
}
```



---

## Support
To report bugs, open discussions, or request new features, please use the repository's GitHub issue.

---

## Roadmap

- [ ] **Multi-language Integration via IPC and Zero-Copy**: Support implementing the "How to do" layer in other backend languages (e.g., Node.js, Python, Rust, etc), communicating with the Heddle engine via high-performance IPC with zero memory copies.
- [ ] **Native AOT Compilation (`heddle build`)**: Native support to transpile `.he` flows directly to Go and compile optimized, standalone binaries for production.
- [ ] **Stdlib**: Built-in stdlib to build distributed systems.

---

## Contributing
If you wish to contribute to the project, please run the tests and format code before submitting a Pull Request:

```bash
# Run unit and integration tests
make test

# Format Go source code
make fmt

# Run static analyzers (linter)
make lint
```

---

## License
This project is licensed under the [GNU General Public License v3](LICENSE).
