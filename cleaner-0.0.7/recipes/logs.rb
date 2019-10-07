# Do not keep gcloud logs.
directory '/root/.config/gcloud/logs' do
  recursive true
  action :delete
end
