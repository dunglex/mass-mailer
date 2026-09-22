package main

import "embed"

//go:embed templates/*.gohtml
var templateFiles embed.FS

var emailTemplates = []EmailTemplate{
	{
		Name:    "Announcement",
		Subject: "An update from us, {{name}}",
		Body:    "<h1>Hello {{name}},</h1><p>We have an update to share with you.</p><p>Thank you,<br>Your team</p>",
	},
	{
		Name:    "Event reminder",
		Subject: "Reminder: your event is coming up, {{name}}",
		Body:    "<h1>Hi {{name}},</h1><p>This is a friendly reminder about our upcoming event.</p><p>We look forward to seeing you.</p><p>Your team</p>",
	},
	{
		Name:    "Follow-up",
		Subject: "Following up, {{name}}",
		Body:    "<h1>Hi {{name}},</h1><p>We wanted to follow up on our last message.</p><p>Please reply if you have any questions.</p><p>Best,<br>Your team</p>",
	},
}
