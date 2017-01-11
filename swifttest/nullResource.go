package swifttest

// nullResource has error stubs for all resource methods.
type nullResource struct{}

func notAllowed() interface{} {
	fatalf(400, "MethodNotAllowed", "The specified method is not allowed against this resource")
	return nil
}

func notAuthorized() interface{} {
	fatalf(401, "Unauthorized", "This server could not verify that you are authorized to access the document you requested.")
	return nil
}

func (nullResource) put(a *action) interface{}    { return notAllowed() }
func (nullResource) get(a *action) interface{}    { return notAllowed() }
func (nullResource) post(a *action) interface{}   { return notAllowed() }
func (nullResource) delete(a *action) interface{} { return notAllowed() }
func (nullResource) copy(a *action) interface{}   { return notAllowed() }
