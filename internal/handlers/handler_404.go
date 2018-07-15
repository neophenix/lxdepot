package handlers

import (
	"bytes"
	"fmt"
	"net/http"
)

// FourOhFourHandler is our 404 response
func FourOhFourHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	tmpl := readTemplate("404.tmpl")

	var out bytes.Buffer
	tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
		"Page": "none",
	})

	fmt.Fprintf(w, string(out.Bytes()))
}
