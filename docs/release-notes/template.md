<!--
Release notes template.
Copy this file in to a file named $patcch-version.md (e.g v1.2.md), replace variables with the correct information, and push!
We should maintain one file for each minor version release, patch releases should be prepended to the files for each minor version affected.
All dates are in RFC-3339 format.
-->
# Release ${minor-version}

<!--
Repeat this for each patch release.
Changelogs should be ordered from newest to oldest, with the most recent patch appearing first.
The version should link to the github tagged release.
-->
## Changelog for [${patch-version}](https://github.com/improbable-eng/etcd-cluster-operator/releases/${patch-version}) - ${date}

<!--
Notable changes include new functionality, features, or API additions.
Do not include minor bugfixes or refactors here.
-->
### Notable changes

<!--
Each change should have a summary, possibly inspired by a commit/ticket title, and a more detailed description of why the change was required and what it enables users to do.
The summary should begin with a link to the ticket tracking the change, or a PR with its implementation.
The description should link to any key issues/pull requests that contributed to the feature's implementation.
Repeat this for each notable change.
-->
* [#${issue-ref-1}](https://github.com/improbable-eng/etcd-cluster-operator/issues/${issue-ref-1}) **${issue-summary-1}**

   Lorem ipsum dolor sit amet, consectetur adipiscing elit. Etiam lacinia eros vel posuere gravida. 
   Pellentesque in tempus risus, at dictum lorem. 
   Sed id tristique arcu. 
   Aliquam fermentum, sapien in lobortis rhoncus, turpis lectus elementum urna, vel convallis nibh augue vitae enim. 
   Proin bibendum nisi a ante euismod bibendum.

   Nunc vitae tristique magna. 
   Sed porta at massa id rhoncus. 
   Duis a auctor leo, id elementum nisi. 
   Maecenas sollicitudin ornare orci a consequat. 
   Duis eu dictum felis. 
   Nunc vitae justo et quam sagittis scelerisque.
   
* [#${issue-ref-2}](https://github.com/improbable-eng/etcd-cluster-operator/issues/${issue-ref-2}) **${issue-summary-2}**

   Lorem ipsum dolor sit amet, consectetur adipiscing elit. 
   Nulla eget eleifend tellus. 
   Maecenas blandit sed nulla at interdum. 
   Vivamus ut massa nisl. 
   Praesent viverra est at metus pulvinar, eu luctus elit condimentum. 
   In tincidunt nibh arcu, id laoreet metus rutrum in. 
   Integer nec lobortis urna, et condimentum felis. 
   Suspendisse elit urna, pretium in egestas vitae, volutpat a purus.

<!--
This section includes a the rest of the changes in this release that are not notable.
This could include bugfixes, documentation updates, or minor features.
-->
### Everything else

<!--
Just include a summary here, no detailed description required.
-->
* [#${issue-ref-1}](https://github.com/improbable-eng/etcd-cluster-operator/issues/${issue-ref-1}) **${issue-summary-1}**
* [#${issue-ref-2}](https://github.com/improbable-eng/etcd-cluster-operator/issues/${issue-ref-2}) **${issue-summary-2}**
* [#${issue-ref-3}](https://github.com/improbable-eng/etcd-cluster-operator/issues/${issue-ref-3}) **${issue-summary-3}**

