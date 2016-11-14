package addon

import "html/template"

var mysqlcfgTmpl = template.Must(template.New("config").Parse(`# https://koli.io/docs/addons
[mysqld]
max_connections = 128
`))
