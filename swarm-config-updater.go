package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("usage:", os.Args[0], "FROM_CONFIG", "TO_CONFIG")
		return
	}

	cli, err := client.NewClientWithOpts(client.FromEnv)

	if err != nil {
		panic(err)
	}

	from := guessConfig(cli, os.Args[1])
	to := guessConfig(cli, os.Args[2])

	// first, swipe to the next configuration
	services := swipeConfig(cli, from, to)
	fmt.Println("update from", from.Spec.Name, "to", to.Spec.Name, "configuration services", services)

	// now, delete the old one
	err = cli.ConfigRemove(context.Background(), from.ID)

	if err != nil {
		panic(err)
	}
	fmt.Println("remove", from.Spec.Name, "configuration")

	// we can duplicate the `to` on the `from`
	from.Spec.Data = to.Spec.Data
	created, err := cli.ConfigCreate(context.Background(), from.Spec)

	if err != nil {
		panic(err)
	}
	fmt.Println("duplicate and rename", to.Spec.Name, "configuration to", from.Spec.Name)

	from = guessConfig(cli, created.ID)

	// and finally switch back
	services = swipeConfig(cli, to, from)
	fmt.Println("update from", from.Spec.Name, "to", to.Spec.Name, "configuration services", services)
}

func guessConfig(cli *client.Client, name string) swarm.Config {
	config, _, err := cli.ConfigInspectWithRaw(context.Background(), name)

	if err != nil {
		panic(err)
	}

	return config
}

// swipe configuration from `from` to `to`
func swipeConfig(cli *client.Client, from swarm.Config, to swarm.Config) []string {
	services, err := cli.ServiceList(context.Background(), types.ServiceListOptions{})

	if err != nil {
		panic(err)
	}

	var updatedServices []string

	for _, service := range services {
		var isChanged bool

		configs := service.Spec.TaskTemplate.ContainerSpec.Configs
		spec := service.Spec

		for index, config := range configs {
			if config.ConfigID != from.ID {
				continue
			}

			config.ConfigID = to.ID
			config.ConfigName = to.Spec.Name

			spec.TaskTemplate.ContainerSpec.Configs[index] = config
			isChanged = true
		}

		if !isChanged {
			continue
		}

		// replace service configuration file
		_, err := cli.ServiceUpdate(
			context.Background(),
			service.ID,
			service.Meta.Version,
			spec,
			types.ServiceUpdateOptions{})

		if err != nil {
			panic(err)
		}

		updatedServices = append(updatedServices, service.ID)
	}

	return updatedServices
}
