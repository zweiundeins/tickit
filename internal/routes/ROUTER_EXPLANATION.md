# Router Explanation: Chi vs Gorilla Mux

## What is `mux`?

In the context of Go web development, `mux` typically refers to **Gorilla Mux** (`github.com/gorilla/mux`), which is a popular HTTP router and URL matcher for building REST APIs and web applications.

However, your project uses **Chi Router** (`github.com/go-chi/chi/v5`), which is a different (and more modern) HTTP router.

## The Confusion

During the implementation of the user routes, there was some confusion where **Gorilla Mux** was incorrectly imported and used instead of **Chi Router**. This has been corrected.

## Comparison: Chi vs Gorilla Mux

### Chi Router (What Your Project Uses)

```go
import "github.com/go-chi/chi/v5"

// Route registration
r.Get("/users/{userId}", getUserHandler)
r.Post("/users", createUserHandler)
r.Route("/api", func(r chi.Router) {
    r.Get("/users", listUsersHandler)
})

// URL parameter extraction
userID := chi.URLParam(r, "userId")

// Middleware
r.Use(middleware.Logger)
r.Group(func(r chi.Router) {
    r.Use(authMiddleware)
    r.Get("/protected", protectedHandler)
})
```

### Gorilla Mux (What Was Incorrectly Used)

```go
import "github.com/gorilla/mux"

// Route registration
r.HandleFunc("/users/{userId}", getUserHandler).Methods("GET")
r.HandleFunc("/users", createUserHandler).Methods("POST")

// URL parameter extraction
vars := mux.Vars(r)
userID := vars["userId"]

// Middleware
r.Use(middlewareFunc)
```

## Key Differences

| Feature | Chi Router | Gorilla Mux |
|---------|------------|-------------|
| **Performance** | Faster, zero allocations | Slower, more allocations |
| **API Style** | Method-based (`r.Get()`) | Generic (`r.HandleFunc().Methods()`) |
| **URL Params** | `chi.URLParam(r, "key")` | `mux.Vars(r)["key"]` |
| **Middleware** | Built-in middleware support | Basic middleware support |
| **Sub-routing** | Excellent with `r.Route()` | Good with sub-routers |
| **Size** | Smaller, lightweight | Larger, more features |
| **Maintenance** | Actively maintained | Less active development |

## Why Chi Router?

Your project uses Chi Router because it offers:

1. **Better Performance**: Zero heap allocations during routing
2. **Cleaner API**: More intuitive method-based routing
3. **Better Middleware**: Built-in middleware chaining and grouping
4. **Modern Design**: Designed with Go's context package in mind
5. **Smaller Footprint**: Lightweight with fewer dependencies

## What Was Fixed

The following incorrect Gorilla Mux usage was corrected:

### Before (Incorrect - Gorilla Mux):
```go
import "github.com/gorilla/mux"

vars := mux.Vars(r)
projectID := vars["projectId"]
```

### After (Correct - Chi Router):
```go
import "github.com/go-chi/chi/v5"

projectID := chi.URLParam(r, "projectId")
```

## Files That Were Corrected

1. **`internal/users/middleware.go`** - Fixed URL parameter extraction
2. **`internal/users/secure_routes.go`** - Fixed imports and router type
3. **`internal/routes/users.go`** - Ensured consistent Chi usage

## Best Practices with Chi Router

### Route Organization
```go
func RegisterRoutes(r chi.Router) {
    // Public routes
    r.Post("/auth/login", loginHandler)
    
    // Protected routes group
    r.Group(func(r chi.Router) {
        r.Use(authMiddleware)
        r.Get("/profile", profileHandler)
        
        // Admin routes sub-group
        r.Group(func(r chi.Router) {
            r.Use(adminMiddleware)
            r.Get("/admin/users", listUsersHandler)
        })
    })
}
```

### Middleware Usage
```go
// Global middleware
r.Use(middleware.Logger)
r.Use(middleware.Recoverer)

// Route-specific middleware
r.With(rateLimitMiddleware).Post("/api/upload", uploadHandler)
```

### URL Parameters
```go
// Route definition
r.Get("/users/{userID}/projects/{projectID}", handler)

// Parameter extraction
func handler(w http.ResponseWriter, r *http.Request) {
    userID := chi.URLParam(r, "userID")
    projectID := chi.URLParam(r, "projectID")
    // ...
}
```

## Conclusion

Chi Router provides a more modern, performant, and developer-friendly API compared to Gorilla Mux. The corrections ensure that your user management routes are properly integrated with the existing Chi-based routing system in your project.