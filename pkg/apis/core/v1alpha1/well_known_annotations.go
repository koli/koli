package v1alpha1

const (
	// AnnotationNamespaceOwner is a string representing the owner of a namespace
	AnnotationNamespaceOwner = "kolihub.io/owner"
	// AnnotationBuild it's a boolean indicating to start a build, after it start the value
	// must be updated to "false"
	AnnotationBuild = "kolihub.io/build"
	// AnnotationBuildRevision is a integer indicating the revision for each new builds, MUST be incremented on
	// each new build
	AnnotationBuildRevision = "kolihub.io/buildrevision"
	// AnnotationAutoDeploy boolean indicating to deploy a new app after the build
	AnnotationAutoDeploy = "kolihub.io/autodeploy" // DEPRECATED
	// AnnotationGitRepository it's a string holding information about the name of the repository, e.g.: owner/repository
	AnnotationGitRepository = "kolihub.io/gitrepository"
	// AnnotationGitRemote it's a string containing information about the remote git repository, e.g.: https://github.com/kolihub/koli
	AnnotationGitRemote = "kolihub.io/gitremote"
	// AnnotationGitRevision it's a string SHA refering to a commit
	AnnotationGitRevision = "kolihub.io/gitrevision"
	// AnnotationGitBranch is the name of the branch to accept webhook requests
	AnnotationGitBranch = "kolihub.io/gitbranch"
	// DEPRECATED, the authToken for each release is populated by a secret
	// the lifecycle of the token is managed by a controller
	// AnnotationAuthToken it's a string credential to communication with the release server
	AnnotationAuthToken = "kolihub.io/authtoken"
	// AnnotationBuildSource it's the source of the request which triggered the build: github (webhook), local, gitstep, etc
	AnnotationBuildSource = "kolihub.io/source"

	AnnotationGitAuthorAvatar  = "kolihub.io/git-author-avatar"
	AnnotationGitAuthorName    = "kolihub.io/git-author-name"
	AnnotationGitCommitMessage = "kolihub.io/git-commit-message"
	AnnotationGitCommitID      = "kolihub.io/git-commit-id"
	AnnotationGitCommitURL     = "kolihub.io/git-commit-url"

	// AnnotationGitCompare information comparing the last commit with the current one
	// https://help.github.com/articles/comparing-commits-across-time/
	AnnotationGitCompare = "kolihub.io/gitcompare"
	// AnnotationGitHubSecretHook contains the webhook secret for validating requests
	AnnotationGitHubSecretHook = "kolihub.io/hook-secret"
	// AnnotationGitHubUser refers to the user who connected the repository
	// the access token of this user will be used to query the GitHub api
	AnnotationGitHubUser = "kolihub.io/gituser"
	// AnnotationSetupStorage it's a boolean indicating to setup the storage onto resources (deploy, statefulset),
	// after the setup finished the value must be turned to "false"
	AnnotationSetupStorage = "kolihub.io/setup-storage"
	// AnnotationApp is used to identify the deployment from a parent resource
	AnnotationApp = "kolihub.io/app"

	// AnnotationSecretLastUpdated informs when a system token was last updated at
	AnnotationSecretLastUpdated = "secret.kolihub.io/updated-at"
)
