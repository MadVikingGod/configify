type {{.nameLower}}Option {{.origType}}
func (o {{.nameLower}}Option) Apply(c *config ) {
	if c.{{.origName}} == nil {
		c.{{.origName}} = {{.origType}}(o)
	}
	for k,v := range o {
		c.{{.origName}}[k] = v
	}
}
func With{{.nameUpper}}(m {{.origType}}) Option { return {{.nameLower}}Option(m) }

