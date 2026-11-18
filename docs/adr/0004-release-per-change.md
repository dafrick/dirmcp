# 0004. Release every change with hand-kept semver and changelog

In the context of one OpenSpec change per PR into `main`, facing the need to know what a version contains, we decided for a `VERSION` file and a Keep a Changelog `CHANGELOG.md` updated in each change and neglected semantic-release and release-please, to achieve one tagged release per merged change with intent written by the author, accepting a manual bump that CI must verify.
