package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/varlink/go/varlink"
	"github.com/varlink/go/varlink/idl"
)

var datadir string = "static"
var templates = template.Must(template.ParseGlob(path.Join(datadir, "*.html")))

func connect(iface string) (*varlink.Connection, error) {
	r, err := varlink.NewResolver("")
	if err != nil {
		return nil, err
	}
	defer r.Close()

	address, err := r.Resolve(iface)
	if err != nil {
		return nil, err
	}

	return varlink.NewConnection(address)
}

func jsonError(writer http.ResponseWriter, message string, code int) {
	type Error struct{ Name string }
	writer.WriteHeader(code)
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")

	err := Error{"org.varlink.http"}
	json.NewEncoder(writer).Encode(err)
}

func serveStaticFile(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		// safe, because this function is only called for a few whitelisted file names
		http.ServeFile(writer, request, path.Join(datadir, request.URL.Path))

	default:
		http.Error(writer, "Method not allowed on this URL", http.StatusMethodNotAllowed)
	}
}

func serveRoot(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.Error(writer, "Not found", http.StatusNotFound)
		return
	}

	switch request.Method {
	case http.MethodGet:
		type info struct {
			Vendor     string
			Product    string
			Version    string
			URL        string
			Interfaces []string
		}
		r, err := varlink.NewResolver(varlink.ResolverAddress)
		if err != nil {
			http.Error(writer, "Not found", http.StatusNotFound)
			return
		}
		defer r.Close()

		var i info
		err = r.GetInfo(&i.Vendor, &i.Product, &i.Version, &i.URL, &i.Interfaces)
		if err != nil {
			http.Error(writer, "Not found"+err.Error(), http.StatusNotFound)
			return
		}

		if strings.Contains(request.Header.Get("Accept"), "application/json") {
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(writer).Encode(i)
		} else {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			templates.ExecuteTemplate(writer, "index.html", i)
		}

	case http.MethodPost:
		type call struct {
			Method     string
			Parameters interface{}
		}
		var in call
		err := json.NewDecoder(request.Body).Decode(&in)
		if err != nil {
			jsonError(writer, err.Error(), http.StatusBadRequest)
			return
		}

		parts := strings.Split(in.Method, ".")
		iface := strings.TrimSuffix(in.Method, "."+parts[len(parts)-1])

		c, err := connect(iface)
		if err != nil {
			if verr, ok := err.(*varlink.Error); ok {
				if verr.Name == "org.varlink.resolver.InterfaceNotFound" {
					writer.WriteHeader(http.StatusNotFound)
					writer.Header().Set("Content-Type", "application/json; charset=utf-8")
					json.NewEncoder(writer).Encode(verr)
					return
				}
			}
			jsonError(writer, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer c.Close()

		type reply struct {
			Parameters interface{} `json:"parameters,omitempty"`
		}
		var out reply
		err = c.Call(in.Method, in.Parameters, &out.Parameters)
		if err != nil {
			jsonError(writer, "Internal server error", http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(writer).Encode(out)

	default:
		if strings.Contains(request.Header.Get("Accept"), "application/json") {
			jsonError(writer, "Bad request", http.StatusBadRequest)
			return
		} else {
			http.Error(writer, "Bad request", http.StatusBadRequest)
			return
		}
	}
}

func defaultValue(i *idl.IDL, t *idl.Type) interface{} {
	switch t.Kind {
	case idl.TypeBool:
		return false

	case idl.TypeInt:
		return 0

	case idl.TypeFloat:
		return 0.0

	case idl.TypeString:
		return ""

	case idl.TypeArray:
		return make([]interface{}, 0)

	case idl.TypeStruct:
		v := make(map[string]interface{})
		for _, field := range t.Fields {
			v[field.Name] = defaultValue(i, field.Type)
		}
		return v

	case idl.TypeAlias:
		for _, alias := range i.Aliases {
			if alias.Name == t.Alias {
				return defaultValue(i, alias.Type)
			}
		}
		return nil
	}

	return nil
}

func serveInterface(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimSuffix(request.URL.Path[len("/interface/"):], "/")
	parts := strings.Split(path, "/")
	name := strings.TrimSuffix(parts[0], ".varlink")

	c, err := connect(name)
	if err != nil {
		if verr, ok := err.(*varlink.Error); ok {
			if verr.Name == "org.varlink.resolver.InterfaceNotFound" {
				http.Error(writer, "Interface does not exist: "+parts[0], http.StatusNotFound)
				return
			}
		}
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		log.Print(err.Error())
		return
	}
	defer c.Close()

	desc, err := c.GetInterfaceDescription(name)
	if err != nil {
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		log.Print(err.Error())
		return
	}

	i, err := idl.New(desc)
	if err != nil {
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		log.Print(err.Error())
		return
	}

	switch len(parts) {
	case 1:
		if strings.HasSuffix(parts[0], ".varlink") {
			writer.Header().Set("Content-Type", "text/plain")
			io.WriteString(writer, i.Description)
		} else {
			templates.ExecuteTemplate(writer, "interface.html", i)
		}
	case 2:
		var method *idl.Method

		for _, m := range i.Methods {
			if m.Name == parts[1] {
				method = m
				break
			}
		}
		if method == nil {
			http.Error(writer, "Method does not exist: "+parts[1], http.StatusNotFound)
			return
		}

		value, err := json.MarshalIndent(defaultValue(i, method.In), "", "  ")
		if err != nil {
			http.Error(writer, "Internal server error", http.StatusInternalServerError)
			log.Print(err.Error())
			return
		}

		templates.ExecuteTemplate(writer, "method.html", map[string]interface{}{
			"Interface":     i,
			"Method":        method,
			"DefaultInArgs": string(value),
		})
	default:
		http.Error(writer, "Bad Request", http.StatusBadRequest)
		return
	}
}

func main() {
	http.HandleFunc("/favicon.ico", serveStaticFile)
	http.HandleFunc("/varlink.css", serveStaticFile)
	http.Handle("/index.html", http.RedirectHandler("/", http.StatusMovedPermanently))

	http.HandleFunc("/interface/", serveInterface)
	http.HandleFunc("/", serveRoot)

	if _, ok := os.LookupEnv("LISTEN_FDS"); ok {
		f := os.NewFile(3, "listen-fd")
		listener, err := net.FileListener(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid listen fd: "+err.Error())
		}

		http.Serve(listener, nil)
	} else {
		if len(os.Args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: %s ADDRESS:PORT\n", os.Args[0])
			os.Exit(1)
		}

		http.ListenAndServe(os.Args[1], nil)
	}
}
