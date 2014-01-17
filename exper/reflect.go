package main

import (
	"log"
	"reflect"
	"strings"
	"strconv"
)

type A struct {
	cityid int
}

type B struct {
	a A
	c *A
}

func match(input interface {}, filter map[string]string) bool {
	v := reflect.ValueOf(input)

	for name, value := range filter {
		parts := strings.Split(name, ".")
		for i, p := range parts {

			if v.Kind() == reflect.Ptr {
				v = reflect.Indirect(v)
			}

			if !v.IsValid() {
				break
			}

			v = v.FieldByName(p)
			if i == len(parts) - 1 {
				switch v.Kind(){
				case reflect.Int:
					if intv, err := strconv.Atoi(value); err == nil {
						return v.Int() == int64(intv)
					}
				case reflect.String:
					return v.String() == value

				}
				// default to false
				return false
			}
		}
	}

	// false if go this far
	return false

}

func main() {

	filter := map[string]string {
		"a.cityid": "1",
	}

	b := &B{
		a: A{cityid: 1},
	}

	log.Println(match(b, filter))


	filter = map[string]string {
		"c.cityid": "1",
	}
	log.Println(match(b, filter))


	//	log.Println(v)

}
