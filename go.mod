module github.com/fanyang89/media-collector

go 1.24.2

replace github.com/CuteReimu/bilibili/v2 => ./sdk/bilibili

require (
	github.com/CuteReimu/bilibili/v2 v2.2.1
	github.com/cockroachdb/errors v1.12.0
	github.com/flytam/filenamify v1.2.0
	github.com/go-resty/resty/v2 v2.16.5
	github.com/k0kubun/go-ansi v0.0.0-20180517002512-3bf9e2903213
	github.com/schollz/progressbar/v3 v3.18.0
	github.com/urfave/cli/v3 v3.3.3
	github.com/xuri/excelize/v2 v2.9.1
	go.uber.org/zap v1.27.0
	golang.org/x/net v0.41.0
	golang.org/x/time v0.12.0
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.30.0
	moul.io/zapgorm2 v1.3.0
)

require (
	github.com/Baozisoftware/qrcode-terminal-go v0.0.0-20170407111555-c0650d8dff0f // indirect
	github.com/cockroachdb/logtags v0.0.0-20241215232642-bb51bb14a506 // indirect
	github.com/cockroachdb/redact v1.1.6 // indirect
	github.com/getsentry/sentry-go v0.33.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.28 // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.4 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	github.com/spf13/cast v1.9.2 // indirect
	github.com/tiendc/go-deepcopy v1.6.0 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/nfp v0.0.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/text v0.26.0 // indirect
)
