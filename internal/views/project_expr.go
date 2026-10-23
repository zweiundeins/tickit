package views

// The expressions of the projects page and of a project's settings. Each
// reads the project's key from the page (data-project) and ids from its
// element: no person's value goes into an expression (NFR-13).

const settingsProject = `el.closest('.tk-settings').dataset.project`

var (
	// The create dialog (FR-02): the key in capitals as typed, a refusal
	// beside its field.
	upperKey             = `$_np.key = $_np.key.toUpperCase().replace(/[^A-Z]/g, '').slice(0, 6)`
	createProjectSubmit  = `$_np.err.name = ''; $_np.err.key = ''; $_np.err.description = ''; $_np.err.any = ''; ` + Act("_np.op", "/cmd/projects/create", "name: $_np.name, key: $_np.key, description: $_np.description")
	createProjectSettled = Settle("_np.op", "o.ok || (o.field === 'key' ? ($_np.err.key = o.msg) : o.field === 'name' ? ($_np.err.name = o.msg) : o.field === 'description' ? ($_np.err.description = o.msg) : ($_np.err.any = o.msg))")
	createProjectFailed  = `evt.detail.el === el && ` + Ended + ` && ($_np.op = '', $_np.err.any = ` + problem + `)`

	saveDetails    = Act("_pd.op", "/cmd/projects/edit", "project: "+settingsProject+", name: $_pd.name, description: $_pd.description, lead: $_pd.lead")
	detailsSettled = Settle("_pd.op", "$_pd.msg = o.ok ? 'Saved.' : o.msg, $_pd.field = o.ok ? '' : (o.field || '')")

	archive        = Act("_ar.op", "/cmd/projects/archive", "project: "+settingsProject+", archived: el.dataset.archived === 'true'")
	archiveSettled = Settle("_ar.op", "o.ok ? location.reload() : ($_ar.msg = o.msg)")

	setRole        = Act("_ms.op", "/cmd/projects/member", "project: "+settingsProject+", person: el.dataset.person, role: el.value")
	removeMember   = Act("_ms.op", "/cmd/projects/member/remove", "project: "+settingsProject+", person: el.dataset.person")
	addMember      = `$_ms.person && ` + Act("_ms.op", "/cmd/projects/member", "project: "+settingsProject+", person: $_ms.person, role: $_ms.role")
	membersSettled = Settle("_ms.op", "$_ms.msg = o.ok ? 'Saved.' : o.msg")

	addComponent      = `$_cs.name.trim() && ` + Act("_cs.op", "/cmd/projects/component", "project: "+settingsProject+", name: $_cs.name, description: $_cs.description")
	removeComponent   = Act("_cs.op", "/cmd/projects/component/remove", "project: "+settingsProject+", name: el.dataset.name")
	componentsSettled = Settle("_cs.op", "o.ok ? ($_cs.msg = 'Saved.', $_cs.name = '', $_cs.description = '') : ($_cs.msg = o.msg)")
)
