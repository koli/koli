package addon

import "html/template"

var rediscfgTmpl = template.Must(template.New("config").Parse(`# https://raw.githubusercontent.com/antirez/redis/3.2/redis.conf
# put your config parameters below, mind the indentation!
databases 1
`))
