# Repository instructions

- For requested code changes in this repository, commit the intended changes to `main`, push them to `origin`, and verify the production deployment triggered by `.github/workflows/deploy.yml`. Keep unrelated working-tree changes out of the commit.

- Use the SER8 Sonder instance (`sonder` Incus container on `sdolbec-ser8.tailf80972.ts.net`, public site `https://sonder.stoverparc.org`) for runtime work and production verification. Do not start, build, or test a local macOS Sonder runtime. Local source checkouts may be used for editing and Git operations. Keep local Sonder launch services disabled.
