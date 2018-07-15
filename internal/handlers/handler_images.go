package handlers

import (
	"bytes"
	"fmt"
	"github.com/neophenix/lxdepot/internal/lxd"
	"log"
	"net/http"
)

// ImageListHandler handles requests for /images
func ImageListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	images, err := lxd.GetImages("")
	if err != nil {
		log.Printf("Could not get image list %s\n", err.Error())
	}

	tmpl := readTemplate("image_list.tmpl")

	var out bytes.Buffer
	tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
		"Page":   "images",
		"Images": images,
	})

	fmt.Fprintf(w, string(out.Bytes()))
}
