type {{.nameLower}}Option struct { {{.origType}} }
func (o {{.nameLower}}Option) Apply(c *config ) { c.{{.origName}} = {{.origType}}(&o) }
func With{{.nameUpper}}(m {{.origType}}) Option { return {{.nameLower}}Option{m} }
