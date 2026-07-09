package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Handler struct{}

type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type AuthResponse struct {
	AccessToken string `json:"access_token"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware)

			r.Get("/users", h.ListUsers)
			r.Post("/users", h.CreateUser)
			r.Get("/users/{id}", h.GetUser)
			r.Put("/users/{id}", h.UpdateUser)
			r.Delete("/users/{id}", h.DeleteUser)
		})

		// Ping godoc
		// @Summary Ping
		// @Description ping
		// @Tags ping
		// @Produce json
		// @Success 200
		// @Router /api/v1/ping [get]
		r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pong"))
		})
	})

	return r
}

// Register godoc
// @Summary Register user
// @Description Register a new user account
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Register request"
// @Success 201 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Router /auth/register [post]
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}
	resp := AuthResponse{AccessToken: "token"}
	writeJSON(w, http.StatusCreated, resp)
}

// Login godoc
// @Summary Login user
// @Description Authenticate user and return access token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login request"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Router /auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}
	resp := AuthResponse{AccessToken: "token"}
	writeJSON(w, http.StatusOK, resp)
}

// ListUsers godoc
// @Summary List users
// @Description list users
// @Tags users
// @Produce json
// @Param page query int false "page"
// @Param limit query int false "limit"
// @Success 200 {array} UserResponse
// @Security BearerAuth
// @Router /api/v1/users [get]
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Query().Get("page")
	limit := r.URL.Query().Get("limit")
	_ = page
	_ = limit
	users := []UserResponse{{ID: 1, Email: "user@example.com"}}
	writeJSON(w, http.StatusOK, users)
}

// CreateUser godoc
// @Summary Create user
// @Description create user
// @Tags users
// @Accept json
// @Produce json
// @Param request body CreateUserRequest true "Request body"
// @Success 201 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/users [post]
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}
	resp := UserResponse{ID: 1, Email: req.Email}
	writeJSON(w, http.StatusCreated, resp)
}

// GetUser godoc
// @Summary Get user
// @Description get user
// @Tags users
// @Produce json
// @Param id path int true "id"
// @Success 200 {object} UserResponse
// @Security BearerAuth
// @Router /api/v1/users/{id} [get]
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	_ = id
	user := UserResponse{ID: 1, Email: "user@example.com"}
	writeJSON(w, http.StatusOK, user)
}

// UpdateUser godoc
// @Summary Update user
// @Description update user
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "id"
// @Param request body CreateUserRequest true "Request body"
// @Success 200 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/users/{id} [put]
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	resp := UserResponse{ID: 1, Email: req.Email}
	writeJSON(w, http.StatusOK, resp)
}

// DeleteUser godoc
// @Summary Delete user
// @Description delete user
// @Tags users
// @Produce json
// @Param id path int true "id"
// @Success 204
// @Security BearerAuth
// @Router /api/v1/users/{id} [delete]
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// ExistingSwagger godoc
// @Summary Existing summary
// @Router /existing [get]
func (h *Handler) ExistingSwagger(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	_ = &Handler{}
}
