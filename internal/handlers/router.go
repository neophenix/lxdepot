package handlers

import(
    "regexp"
    "net/http"
)

type Route struct {
    Regex *regexp.Regexp
    Handler func(w http.ResponseWriter, r *http.Request)
}
var Routes []Route

func AddRoute(regex string, f func(w http.ResponseWriter, r *http.Request)) {
     Routes = append(Routes, Route{Regex: regexp.MustCompile(regex), Handler: f})
}

func GetRouteHandler(path string) func(w http.ResponseWriter, r *http.Request) {
    for _, route := range Routes {
        if route.Regex.MatchString(path) {
            return route.Handler
        }
    }

    return nil
}
