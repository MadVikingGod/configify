type {{.nameLower}}Option {{.origType}}
func (o {{.nameLower}}Option) Apply(c *config ) { c.{{.origName}} = append(c.{{.origName}}, {{.origType}}(o)...) }
func With{{.nameUpper}}(m {{.origType}}) Option { return {{.nameLower}}Option(m) }
