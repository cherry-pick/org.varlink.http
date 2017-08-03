package varlink

type Service struct {
	Name        string
	Description string
	Properties  []Property
	Interfaces  []string
}

type Property struct {
	Name  string `json:"key"`
	Value string
}
