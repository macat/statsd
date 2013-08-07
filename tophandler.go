package main

var topHandler = PrefixRouter(map[string]Handler{
	"/":       nil,
	"/static": nil,
	"/users":  usersRouter,
	"/groups": groupsRouter,
})
