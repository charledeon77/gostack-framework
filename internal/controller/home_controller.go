package controller

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/charledeon77/gostack-framework/framework/http"
)

// HomeController serves as the primary controller for home-related 
// application features. It encapsulates the methods required to 
// fulfill user requests for these specific endpoints.
type HomeController struct {
	DB contract.Database
}

// NewHomeController initializes a new HomeController with the required dependencies.
//
// Parameters:
//   - db: An implementation of the contract.Database interface.
func NewHomeController(db contract.Database) *HomeController {
	return &HomeController{
		DB: db,
	}
}

// Index renders the landing page of the application.
func (c *HomeController) Index(ctx *http.Context) {
	if err := ctx.Render("landing", nil); err != nil {
		ctx.Writer.WriteHeader(500)
		_, _ = ctx.Writer.Write([]byte("Error rendering landing page"))
	}
}

// Users processes requests for the user list resource.
// It acts as the final handler for the "/users" endpoint.
//
// Parameters:
//   - ctx: The unified framework HTTP Context wrapper.
func (c *HomeController) Users(ctx *http.Context) {
	fmt.Fprintf(ctx.Writer, "User list requested")
}