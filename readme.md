# Configify
The goal is to create a tool like Stringer which when run like `configify -type config .` with a config of 
```go
type config struct {
	myType MyType
	color  string
	height int
}
```

It creates the `New()`, `Option` interface, the `With*()` functions, and the `Apply()` function

For example

```go
func New(options ...Option) *config {
	cfg := &config{}
	cfg.Apply(options...)
	return cfg
}
func (c *config) Apply(options ...Option) {
	for _, option := range options {
        option.Apply(c)		
    }
}

type Option interface {
	Apply(*config)
}

type myTypeOption MyType
func (o myTypeOption) Apply(c *config ) {c.myType = MyType(o)}
func WithMyType(m MyType) Option { return myTypeOption(m)}

type colorOption string
func (o colorOption) Apply(c *config ) {c.color = string(o)}
func WithColor(s string) Option { return colorOption(s)}

type heightOption int
func (o heightOption) Apply(c *config ) {c.height = int(o)}
func WithHeight(i int) Option { return heightOption(i)}
```