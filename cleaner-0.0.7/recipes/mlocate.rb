# Sometimes removal of mlocate fails without this step.
execute 'clean' do
   command 'apt-get -f install --yes'
end

# Clear local repo of retrieved package files.
package "mlocate" do
  action :remove
end
