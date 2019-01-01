# Getters
Getters is a code generator tool for Go. It generates getter methods for
structs with pointer fields.

Getters is a standalone version of `gen-accessors` from
[google/go-github](https://github.com/google/go-github). Aside from it's
standalone, it's customizable too and has more primitive types support such as
floats and uints.

```go
type Pony struct {
	Name   *string
	Age    *int64
}
```

Generates...

```go
func (p *Pony) GetAge() int64 {
	if p == nil || p.Age == nil {
		return 0
	}
	return *p.Age
}

func (p *Pony) GetName() string {
	if p == nil || p.Name == nil {
		return ""
	}
	return *p.Name
}
```

## Install
```
go get -u github.com/tors/getters
```

## Usage

### Command line

```bash
# Basic usage
# Looks for .getters.yml. If it doesn't exist, this will generate
# getter methods for all struct pointer fields.
getters

# Checks custom_getters.yml with verbose mode on
getters -v custom_getters.yml

# Merge and override options
getters -v -s Pony,Kitten -m Horse.GetName,Cat.GetAge

# Ignore main packages
getters -p main
```

### Generators
```go
//go:generate getters
```

```go
//go:generate getters -v -s Pony,Kitten -m Horse.GetName,Cat.GetAge
```

### YAML config
```yaml
suffix: _generated.go
ignore_structs:
  - Kitten
ignore_methods:
  - Pony.GetWeight
ignore_packages:
  - main
```
