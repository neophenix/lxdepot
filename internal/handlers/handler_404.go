package handlers

import(
    "fmt"
    "bytes"
    "net/http"
)

func FourOhFourHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")

    tmpl := readTemplate("404.tmpl")

    var out bytes.Buffer
    tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
        "Page": "none",
    })

    fmt.Fprintf(w, string(out.Bytes()))
}
