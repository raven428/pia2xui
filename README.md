# PIA2XUI

Update 3x-ui WireGuard outbound from the PIA account

`curl -s https://serverlist.piaservers.net/vpninfo/servers/v6 | jqc1 | less` for regions

## Build steps
* clone me:

  ```shell
  git clone --recursive \
  git@github.com:raven428/pia2xui.git \
  pia2xui
  ```

* commit and push changes
* choose tag to release:

  ```shell
    export VER=v000 && git checkout master && git pull
  ```

* perform something of bellow:
  * `make local` - build on local machine
  * `make gh-act` - send to GitHub actions

* go to [releases](../../releases) and publish the recent draft after finish