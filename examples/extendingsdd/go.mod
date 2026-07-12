module example.com/extendingsdd

go 1.26.1

require github.com/networkteam/sdd v0.0.0

require (
	golang.org/x/mod v0.35.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/networkteam/sdd => ../..
