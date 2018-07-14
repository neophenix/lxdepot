package handlers

import (
    "log"
    "net/http"
)

func RootHandler(w http.ResponseWriter, r *http.Request) {
    log.Println(r.Method, r.URL.Path, r.RemoteAddr)
    handler := GetRouteHandler(r.URL.Path)
    if handler != nil {
        handler(w,r)
        return
    }

    FourOhFourHandler(w,r)
}
