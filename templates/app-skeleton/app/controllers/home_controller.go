package controllers

import (
	"github.com/charledeon77/gostack-framework/framework/http"
)

// HomeController handles base landing requests.
type HomeController struct{}

// NewHomeController creates a new instance.
func NewHomeController() *HomeController {
	return &HomeController{}
}

// Index renders the application welcome view.
func (c *HomeController) Index(ctx *http.Context) error {
	return ctx.JSON(200, map[string]any{
		"message": "Welcome to GoStack!",
		"status":  "running",
	})
}

