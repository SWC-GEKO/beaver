package controlplane

import (
	"log"
	"net/http"
)

func UploadFunction(rw http.ResponseWriter, r *http.Request) {
	log.Println(r.Body)
}
