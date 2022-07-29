# Docker Compose to Nomad

## Roadmap

- [ ] Import docker libs and tranlates docker-compse to nomad hcl

  - [x] YAML -> compose.Project
  - [ ] compose.Project -> nomad.Job
  - [ ] nomad.Job -> HCL

- [ ] Snazzy UI to attach our very large audience
- [ ] Nice to have: convert from nomad hcl to docker-compose
- [ ] Setup github to static host
- [ ] CI/CD with Drone
- [ ] Come up with a name and domain

## Guiding Principles

- 1 docker-compose file = 1 job
