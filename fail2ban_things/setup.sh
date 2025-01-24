#!/user/bin/sh

sudo cp -a ./filters/. /etc/fail2ban/filter.d/
sudo cp -a ./jails/. /etc/fail2ban/jail.d/