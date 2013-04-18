package provision

type Deployer interface {
    Deploy(App) error
}
