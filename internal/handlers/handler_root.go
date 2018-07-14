package handlers

import (
    "log"
    "net/http"
)

// RootHandler handles requests for everything, and then compares the requested URL
// to our array of routes, the first match wins and we call that handler.  Requests
// for / are shown a container list, anything not found is 404'd
func RootHandler(w http.ResponseWriter, r *http.Request) {
    log.Println(r.Method, r.URL.Path, r.RemoteAddr)
    handler := GetRouteHandler(r.URL.Path)
    if handler != nil {
        handler(w,r)
        return
    }

    // Special case if we go to just /
    if r.URL.Path == "/" {
        ContainerListHandler(w,r)
        return
    }

    FourOhFourHandler(w,r)
}
