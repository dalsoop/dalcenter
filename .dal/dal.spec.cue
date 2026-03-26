#Player: "claude" | "codex" | "gemini"
#Role:   "leader" | "member"
#DalProfile: {
	uuid!:    string & != ""
	name!:    string & != ""
	version!: string
	player!:  #Player
	role!:    #Role
	skills?:  [...string]
	hooks?:   [...string]
}
