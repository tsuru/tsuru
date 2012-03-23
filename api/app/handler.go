package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
	"io/ioutil"
	"net/http"
	"os"
	"time"
	//"io"
	"log"
	"os/exec"
)

func Upload(w http.ResponseWriter, r *http.Request) {
	app := App{Name: r.URL.Query().Get(":name")}
	app.Get()

	if app.Id == 0 {
		http.NotFound(w, r)
	} else {
		f, _, err := r.FormFile("application")
		if err != nil {
			panic(err)
		}

		releaseName := time.Now().Format("20060102150405")
		zipFile := fmt.Sprintf("/tmp/%s.zip", releaseName)
		zipDir := fmt.Sprintf("/tmp/%s", releaseName)

		newFile, err := os.Create(zipFile)
		if err != nil {
			panic(err)
		}
		out, _ := ioutil.ReadAll(f)
		newFile.Write(out)

		cmd := exec.Command("unzip", zipFile, "-d", zipDir)
		output, err := cmd.Output()
		if err != nil {
			panic(err)
		}
		log.Printf(string(output))

		appDir := "/home/application"
		currentDir := appDir + "/releases/current"
		gunicorn := appDir + "/env/bin/gunicorn_django"
		releasesDir := appDir + "/releases"
		releaseDir := releasesDir + "/" + releaseName

		u := unit.Unit{Name: app.Name}
		u.SendFile(zipDir, releaseDir)
		//u.Command(fmt.Sprintf("'rm -rf %s'", currentDir))
		u.Command(fmt.Sprintf("'cd %s && ln -nfs %s current'", releasesDir, releaseName))
		u.Command("'sudo killall gunicorn_django'")
		u.Command(fmt.Sprintf("'cd %s && sudo %s --daemon --workers=3 --bind=127.0.0.1:8888'", currentDir, gunicorn))

		fmt.Fprint(w, "success")
	}
}

func AppList(w http.ResponseWriter, r *http.Request) {
	apps, err := AllApps()
	if err != nil {
		panic(err)
	}

	b, err := json.Marshal(apps)
	if err != nil {
		panic(err)
	}
	fmt.Fprint(w, bytes.NewBuffer(b).String())
}

func AppInfo(w http.ResponseWriter, r *http.Request) {
	app := App{Name: r.URL.Query().Get(":name")}
	app.Get()

	if app.Id == 0 {
		http.NotFound(w, r)
	} else {
		b, err := json.Marshal(app)
		if err != nil {
			panic(err)
		}
		fmt.Fprint(w, bytes.NewBuffer(b).String())
	}

}

func CreateAppHandler(w http.ResponseWriter, r *http.Request) {
	var app App

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(body, &app)
	if err != nil {
		panic(err)
	}

	err = app.Create()
	if err != nil {
		panic(err)
	}
	fmt.Fprint(w, "success")
}
