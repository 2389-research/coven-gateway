# Project Gotchas

- Docker image ownership: build and verify Docker images in cloud CI. Do not install local Docker builder plugins or build images on a developer machine unless Doctor Biz explicitly requests a local diagnostic build.
- CI scope: when an existing cloud release workflow already owns Docker builds, do not expand it into PR validation, permission hardening, or release redesign unless Doctor Biz explicitly asks for that work.
- Root-cause scope: when one build defect affects multiple published artifact paths, repair every affected publisher before declaring the bug releasable. A separate workflow is not a separate bug.
