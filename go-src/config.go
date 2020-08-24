package main

import (
	"os"
	"reflect"
	"strconv"
)

var cfg struct {
	NS      string `env:"NS"`
	RSSvc   string `env:"RS_SVC"`
	MDBUser string `env:"MDB_USER"`
	MDBPass string `env:"MDB_PASS"`
	MDBPort uint16 `env:"MDB_PORT"`
}

func init() {
	cv := reflect.ValueOf(&cfg).Elem()
	ct := cv.Type()

	for i := 0; i < ct.NumField(); i++ {
		fv := cv.Field(i)
		ft := ct.Field(i)

		envvarName, _ := ft.Tag.Lookup("env")
		v := os.Getenv(envvarName)
		if v == "" {
			continue
		}

		switch k := fv.Kind(); k {
		case reflect.String:
			fv.SetString(v)
		case reflect.Uint16:
			vu, err := strconv.ParseUint(v, 10, 16)
			if err != nil {
				panic(err)
			}
			fv.SetUint(vu)
		default:
			panic("unsupported cfg field Kind: " + k.String())
		}
	}
}
