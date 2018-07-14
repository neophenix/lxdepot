package handlers

import(
    "regexp"
    "net/http"
)

// Route holds all our routing rules
type Route struct {
    Regex *regexp.Regexp // a regex to compare the request path to
    Handler func(w http.ResponseWriter, r *http.Request) // a func pointer to call if the regex matches
}
var Routes []Route

// AddRoute compiles the regex string and appends it to our route list with its handler func pointer
func AddRoute(regex string, f func(w http.ResponseWriter, r *http.Request)) {
     Routes = append(Routes, Route{Regex: regexp.MustCompile(regex), Handler: f})
}

// GetRouteHandler compares the path string to the route list and returns the handler pointer if found or nil
func GetRouteHandler(path string) func(w http.ResponseWriter, r *http.Request) {
    for _, route := range Routes {
        if route.Regex.MatchString(path) {
            return route.Handler
        }
    }

    return nil
}
