# Clear local repo of retrieved package files.
execute 'clean' do
  command 'apt clean'
end

# Remove unused package dependencies. We avoid unning at 6am to allow unattended-upgrade to complete.
cron 'autoremove' do
  action :create
  command 'apt autoremove -yf'
  hour '0-5,7-23'
end
