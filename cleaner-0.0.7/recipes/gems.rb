# Remove old aws-sdk versions. These are executed in this order as they depend on each other & will not be cleaned
# up if they are still referred to by old versions of other SDKs.
execute 'autoremove' do
  command '/opt/chef/embedded/bin/gem cleanup aws-sdk'
end

execute 'autoremove' do
  command '/opt/chef/embedded/bin/gem cleanup aws-sdk-resources'
end

execute 'autoremove' do
  command '/opt/chef/embedded/bin/gem cleanup aws-sdk-core'
end
