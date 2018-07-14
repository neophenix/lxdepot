package handlers

import(
    "log"
    "html/template"
    "github.com/neophenix/lxdepot/internal/utils"
)

var WebRoot string
var CacheTemplates bool
var templates = make(map[string]*template.Template)

func readTemplate(filename string) *template.Template {
    if CacheTemplates {
        if tmpl, ok := templates[filename]; ok {
            return tmpl
        }
    }

    funcs := template.FuncMap{
        "MakeBytesMoreHuman": utils.MakeBytesMoreHuman,
        "MakeIntBytesMoreHuman": utils.MakeIntBytesMoreHuman,
    }

    t, err := template.New(filename).Funcs(funcs).ParseFiles(WebRoot + "/templates/base.tmpl", WebRoot + "/templates/" + filename)
    if err != nil {
        log.Fatal("Could not open template: " + WebRoot + "/" + filename + " : " + err.Error())
    }

    templates[filename] = t

    return t
}
