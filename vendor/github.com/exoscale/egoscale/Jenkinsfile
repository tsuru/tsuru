@Library('jenkins-pipeline') _

node {
  cleanWs()

  try {
    dir('src') {
      stage('SCM') {
        checkout scm
      }
      stage('gofmt') {
        gofmt()
      }
      updateGithubCommitStatus('PENDING', "${env.WORKSPACE}/src")
      stage('Build') {
        parallel (
          "golint": {
            golint()
          },
          "go test": {
            test()
          },
          "go install": {
            build()
          }
        )
      }
    }
  } catch (err) {
    currentBuild.result = 'FAILURE'
    throw err
  } finally {
    if (!currentBuild.result) {
      currentBuild.result = 'SUCCESS'
    }
    updateGithubCommitStatus(currentBuild.result, "${env.WORKSPACE}/src")
    cleanWs cleanWhenFailure: false
  }
}

def gofmt() {
  docker.withRegistry('https://registry.internal.exoscale.ch') {
    def image = docker.image('registry.internal.exoscale.ch/exoscale/golang:1.10')
    image.pull()
    image.inside("-u root --net=host -v ${env.WORKSPACE}/src:/go/src/github.com/exoscale/egoscale") {
      sh 'test `gofmt -s -d -e . | tee -a /dev/fd/2 | wc -l` -eq 0'
      // let's not gofmt the dependencies
      sh 'cd /go/src/github.com/exoscale/egoscale && dep ensure -v -vendor-only'
      sh 'cd /go/src/github.com/exoscale/egoscale/cmd/cs && dep ensure -v -vendor-only'
      sh 'cd /go/src/github.com/exoscale/egoscale/cmd/exo && dep ensure -v -vendor-only'
    }
  }
}

def golint() {
  docker.withRegistry('https://registry.internal.exoscale.ch') {
    def image = docker.image('registry.internal.exoscale.ch/exoscale/golang:1.10')
    image.inside("-u root --net=host -v ${env.WORKSPACE}/src:/go/src/github.com/exoscale/egoscale") {
      sh 'golint -set_exit_status github.com/exoscale/egoscale'
      sh 'golint -set_exit_status github.com/exoscale/egoscale/cmd/cs'
      sh 'golint -set_exit_status github.com/exoscale/egoscale/cmd/exo'
      sh 'golint -set_exit_status github.com/exoscale/egoscale/generate'
      sh 'go vet github.com/exoscale/egoscale'
      sh 'go vet github.com/exoscale/egoscale/cmd/cs'
      sh 'go vet github.com/exoscale/egoscale/cmd/exo'
      sh 'go vet github.com/exoscale/egoscale/generate'
    }
  }
}

def test() {
  docker.withRegistry('https://registry.internal.exoscale.ch') {
    def image = docker.image('registry.internal.exoscale.ch/exoscale/golang:1.10')
    image.inside("-u root --net=host -v ${env.WORKSPACE}/src:/go/src/github.com/exoscale/egoscale") {
      sh 'cd /go/src/github.com/exoscale/egoscale && go test -v'
    }
  }
}

def build() {
  docker.withRegistry('https://registry.internal.exoscale.ch') {
    def image = docker.image('registry.internal.exoscale.ch/exoscale/golang:1.10')
    image.inside("-u root --net=host -v ${env.WORKSPACE}/src:/go/src/github.com/exoscale/egoscale") {
      sh 'go install github.com/exoscale/egoscale/cmd/cs'
      sh 'test -e /go/bin/cs'
      sh 'go install github.com/exoscale/egoscale/cmd/exo'
      sh 'test -e /go/bin/exo'
    }
  }
}
