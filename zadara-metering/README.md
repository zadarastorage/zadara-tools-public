# VPSA Metering Analysis Tool

## Introduction

This tool aims to provide an easy to use, portable way to view VPSA metering database exports as dynamic graphs in [Grafana](https://grafana.com/)

## Brief Description of Usage

More details below on how this tool works.  This is a brief checklist of how to make this work.

### Starting
- Install [VirtualBox](https://www.virtualbox.org/) on your computer
- Install [Vagrant](https://www.vagrantup.com/) on your computer
- Clone this repository, or download it's full contents from Github
- **Copy one or several metering database downloads from the VPSA settings page into the `metering_data` sub-directory**
- Open a command prompt on your computer
- Change directory into the code checkout/download from above (i.e. `cd zadara-tools-public/zadara-metering`)
- Run `vagrant up` and wait for initialization and processing to finish
- Inside your web browser, open [http://localhost:3000](http://localhost:3000)

### Closing
- When finished with analysis, the VM can either be shutdown with `vagrant halt` or deleted with `vagrant destroy --force`
- If shutdown, every subsequent time `vagrant up` is called, it will extract and process any new metering data in the `metering_data` directory - but will not re-install and re-configure dependencies - or delete previously processed data
- If deleted, all previously processed data is removed, all dependencies will be reinstalled, and metering analyzed on the next `vagrant up`

## Detail on Design

Based on the instructions in `Vagrantfile`, Vagrant will:

- Download Ubuntu Bionic base box from [Vagrant Cloud](https://app.vagrantup.com/boxes/search)
- Launch Ubuntu Bionic VM in VirtualBox
- Install InfluxDB inside VM
- Create InfluxDB database inside VM
- Install and configure Grafana inside VM
- Import `grafana-dashboard.json` Grafana dashboard inside VM
- Search for all metering_*.zip files in `metering_data` and extract them - these files are found inside the VM at `/vagrant` - which is linked to the local checkout directory by Vagrant
- Convert all SQLite files with `.db` extension into InfluxDB time series with provided Go program: `bin/zadara-metering-linux-amd64`

Some relevant variables are near the top of `Vagrantfile` - namely how many CPUs and how much memory to allocate to the VM (defaults are: 2 CPUs and 2GB memory).  These can be changed to your desired values

## Troubleshooting

If any issue is seen on `vagrant up`, it is recommended any previous VM is destroyed with `vagrant destroy --force` and rebuilt from scratch with `vagrant up`.

If issues persist, please provide all relevant logs/screenshots to [Zadara Support](https://www.zadarastorage.com/company/contact-us/).

## Usage

TODO