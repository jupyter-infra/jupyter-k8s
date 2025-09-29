# Helm Chart Customization

This directory contains scripts and patches for customizing the generated Helm chart.

## How it works

1. The Kubebuilder Helm plugin (`helm/v1-alpha`) generates base Helm chart files in `dist/chart/`
2. When using `--force`, all files are regenerated, overwriting any customizations
3. Our patching system applies customizations after generation:
   - Configurable image pull policies through Helm values
   - Custom args for the controller

## Adding new patches

1. Create a new patch file in `helm-patches/`
2. Run `make helm-generate` to apply patches

## Current customizations

- `values.yaml`: Configurable image pull policy for controller
- Manager args: Uses the image pull policy from Helm values

## Future improvements

Consider upgrading to the `helm/v2-alpha` plugin when available, which has better support for customization.