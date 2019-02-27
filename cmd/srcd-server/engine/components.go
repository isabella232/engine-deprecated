package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/src-d/engine/api"
	"github.com/src-d/engine/docker"
)

const (
	startComponentTimeout = 60 * time.Second
)

// Component to be run.
type Component struct {
	Name         string
	Start        docker.StartFunc
	Dependencies []Component
}

// Run the given components if they're not already running. It will recursively
// run all the component dependencies.
func Run(ctx context.Context, cs ...Component) error {
	return run(ctx, cs, make(map[string]struct{}))
}

func run(ctx context.Context, cs []Component, seen map[string]struct{}) error {
	for _, c := range cs {
		if len(c.Dependencies) > 0 {
			if err := run(ctx, c.Dependencies, seen); err != nil {
				return err
			}
		}

		if _, ok := seen[c.Name]; ok {
			continue
		}

		seen[c.Name] = struct{}{}
		_, err := docker.InfoOrStart(ctx, c.Name, c.Start)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) StartComponent(
	ctx context.Context,
	r *api.StartComponentRequest,
) (*api.StartComponentResponse, error) {
	return &api.StartComponentResponse{}, s.startComponentAtPort(ctx, r.Name, int(r.Port))
}

func (s *Server) StopComponent(
	ctx context.Context,
	r *api.StopComponentRequest,
) (*api.StopComponentResponse, error) {
	return &api.StopComponentResponse{}, docker.RemoveContainer(r.Name)
}

func (s *Server) startComponent(ctx context.Context, name string) error {
	return s.startComponentAtPort(ctx, name, -1)
}

func (s *Server) startComponentAtPort(ctx context.Context, name string, port int) error {
	var err error
	switch name {
	case gitbaseWeb.Name:
		gbComp, err := s.gitbaseComponent()
		if err != nil {
			break
		}

		return Run(ctx, Component{
			Name:         gitbaseWeb.Name,
			Start:        createGitbaseWeb(docker.WithPort(port, gitbaseWebPrivatePort)),
			Dependencies: []Component{*gbComp},
		})
	case bblfshWeb.Name:
		bbfComp, err := s.bblfshComponent()
		if err != nil {
			break
		}

		return Run(ctx, Component{
			Name:         bblfshWeb.Name,
			Start:        createBblfshWeb(docker.WithPort(port, bblfshWebPrivatePort)),
			Dependencies: []Component{*bbfComp},
		})
	case bblfshd.Name:
		bbfComp, err := s.bblfshComponent()
		if err != nil {
			break
		}

		return Run(ctx, *bbfComp)
	case gitbase.Name:
		gbComp, err := s.gitbaseComponent()
		if err != nil {
			break
		}

		return Run(ctx, *gbComp)
	default:
		return fmt.Errorf("can't start unknown component %s", name)
	}

	return errors.Wrapf(err, "can't start component %s", name)
}

func (s *Server) gitbaseComponent() (*Component, error) {
	indexDir := filepath.Join(s.datadir, "gitbase", s.workdirHash)

	workdirHostPath, err := docker.HostPath(s.workdir)
	if err != nil {
		return nil, errors.Wrapf(err, "can't process host path for workdir %s", s.workdir)
	}

	indexDirHostPath, err := docker.HostPath(indexDir)
	if err != nil {
		return nil, errors.Wrapf(err, "can't process host path for indexdir %s", indexDir)
	}

	bblfshComponent, err := s.bblfshComponent()
	if err != nil {
		return nil, errors.Wrapf(err, "can't create %s component", bblfshd.Name)
	}

	return &Component{
		Name: gitbase.Name,
		Start: createGitbase(
			docker.WithSharedDirectory(workdirHostPath, gitbaseMountPath),
			docker.WithSharedDirectory(indexDirHostPath, gitbaseIndexMountPath),
			docker.WithPort(gitbasePort, gitbasePort),
		),
		Dependencies: []Component{*bblfshComponent},
	}, nil
}

func (s *Server) bblfshComponent() (*Component, error) {
	return &Component{
		Name: bblfshd.Name,
		Start: createBbblfshd(
			docker.WithPort(bblfshParsePort, bblfshParsePort),
		),
	}, nil
}
