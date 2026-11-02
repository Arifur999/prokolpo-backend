package auth

import "github.com/Arifur999/prokolpo-backend/internal/msg"

// Error codes the frontend can branch on. The Bangla text is the design's copy.
const (
	codeInvalidInput       = "INVALID_INPUT"
	codePhoneTaken         = "PHONE_TAKEN"
	codeEmailTaken         = "EMAIL_TAKEN"
	codeRegistrationClosed = "REGISTRATION_CLOSED"
	codeInvalidCredentials = "INVALID_CREDENTIALS" //nolint:gosec // an error code name, not a credential
	codeAccountSuspended   = "ACCOUNT_SUSPENDED"
	codeTooManyAttempts    = "TOO_MANY_ATTEMPTS"
	codeSessionInvalid     = "SESSION_INVALID"
	codeUnauthorized       = "UNAUTHORIZED"
	codeAccountNotFound    = "ACCOUNT_NOT_FOUND"
	codeNoEmail            = "NO_EMAIL_ON_ACCOUNT"
	codeResetThrottled     = "RESET_RATE_LIMITED"
	codeResetLinkInvalid   = "RESET_LINK_INVALID"
	codeResetLinkExpired   = "RESET_LINK_EXPIRED"
	codeResetLinkUsed      = "RESET_LINK_USED"
	codeResetCodeInvalid   = "RESET_CODE_INVALID"
	codeResetCodeExpired   = "RESET_CODE_EXPIRED"
	codeResetCodeLocked    = "RESET_CODE_LOCKED"
	codeResetCodeWrong     = "RESET_CODE_WRONG"
	codeNoInvite           = "NO_INVITE"
	codeOwnsProjects       = "OWNS_PROJECTS"
	codeStaffKeep          = "STAFF_ACCOUNT"
	codeDeviceGone         = "DEVICE_NOT_FOUND"
)

var messages = msg.Catalog{
	codeInvalidInput:       {BN: "কিছু তথ্য ঠিক নেই — লাল চিহ্ন দেওয়া ঘরগুলো দেখুন", EN: "Some details need fixing — check the highlighted fields"},
	codePhoneTaken:         {BN: "এই নম্বর দিয়ে আগেই অ্যাকাউন্ট খোলা হয়েছে", EN: "An account already uses this mobile number"},
	codeEmailTaken:         {BN: "এই ইমেইল আগেই ব্যবহার হয়েছে", EN: "This email is already in use"},
	codeRegistrationClosed: {BN: "কিছুদিনের জন্য নতুন রেজিস্ট্রেশন বন্ধ", EN: "New registrations are paused for now"},
	codeInvalidCredentials: {BN: "মোবাইল নম্বর/ইমেইল বা পাসওয়ার্ড মেলেনি", EN: "Mobile number/email or password is incorrect"},
	codeAccountSuspended:   {BN: "আপনার অ্যাকাউন্ট সাময়িকভাবে বন্ধ আছে — সাপোর্টে যোগাযোগ করুন", EN: "Your account is temporarily suspended — please contact support"},
	codeTooManyAttempts:    {BN: "অনেকবার ভুল হয়েছে — 15 মিনিট পরে আবার চেষ্টা করুন", EN: "Too many attempts — try again in 15 minutes"},
	codeSessionInvalid:     {BN: "লগইনের মেয়াদ শেষ — আবার লগইন করুন", EN: "Your session has ended — please log in again"},
	codeUnauthorized:       {BN: "আগে লগইন করুন", EN: "Please log in first"},
	codeAccountNotFound:    {BN: "এই নম্বরে কোনো অ্যাকাউন্ট নেই — নতুন অ্যাকাউন্ট খুলতে পারেন", EN: "No account uses this number — you can create one"},
	codeNoEmail:            {BN: "আপনার অ্যাকাউন্টে কোনো ইমেইল দেওয়া নেই", EN: "Your account has no email address"},
	codeResetThrottled:     {BN: "1 মিনিট পর আবার পাঠাতে পারবেন", EN: "You can send another link in a minute"},
	codeResetLinkInvalid:   {BN: "এই লিংক আর কাজ করবে না — নতুন লিংক নিন", EN: "This link no longer works — request a new one"},
	codeResetLinkExpired:   {BN: "লিংকের মেয়াদ শেষ", EN: "This link has expired"},
	codeResetLinkUsed:      {BN: "এই লিংক আগেই ব্যবহার হয়েছে", EN: "This link was already used"},
	codeResetCodeInvalid:   {BN: "এই অ্যাকাউন্টের জন্য কোনো কোড নেই — মালিকের কাছ থেকে কোড নিন", EN: "There is no code for this account — ask the owner for one"},
	codeResetCodeExpired:   {BN: "কোডের মেয়াদ শেষ — মালিকের কাছ থেকে নতুন কোড নিন", EN: "This code has expired — ask the owner for a new one"},
	codeResetCodeLocked:    {BN: "অনেকবার ভুল হয়েছে — মালিকের কাছ থেকে নতুন কোড নিন", EN: "Too many wrong tries — ask the owner for a new code"},
	codeResetCodeWrong:     {BN: "কোড মেলেনি", EN: "The code didn't match"},
	codeOwnsProjects:       {BN: "আপনি এখনো প্রজেক্টের মালিক — আগে সেগুলো অন্যকে দিন বা মুছুন", EN: "You still own projects — hand them over or delete them first"},
	codeStaffKeep:          {BN: "স্টাফের অ্যাকাউন্ট এখান থেকে মোছা যায় না", EN: "Staff accounts can't be deleted here"},
	codeDeviceGone:         {BN: "এই ডিভাইস আগেই লগআউট হয়েছে", EN: "This device is already logged out"},
	codeNoInvite:           {BN: "এই নম্বরে কোনো আমন্ত্রণ নেই — প্রজেক্টের মালিককে বলুন আপনাকে মেম্বার হিসেবে যোগ করতে, তারপর আবার চেষ্টা করুন", EN: "This number has no invitation — ask the project's owner to add you as a member, then try again"},
}

// Field messages for registration and reset validation.
var fieldMessages = map[string]msg.Text{
	"codeShort":     {BN: "পুরো ৬ অঙ্কের কোড দিন", EN: "Enter all 6 digits of the code"},
	"codeWrong":     {BN: "কোড মেলেনি — আর %d বার চেষ্টা করা যাবে", EN: "The code didn't match — %d tries left"},
	"companyShort":  {BN: "কোম্পানির নাম কমপক্ষে ২ অক্ষরের হতে হবে", EN: "Company name must be at least 2 characters"},
	"companyLong":   {BN: "কোম্পানির নাম ৮০ অক্ষরের বেশি হতে পারবে না", EN: "Company name can't be longer than 80 characters"},
	"firstLong":     {BN: "প্রথম নাম ৩০ অক্ষরের বেশি হতে পারবে না", EN: "First name can't be longer than 30 characters"},
	"lastLong":      {BN: "শেষ নাম ৩০ অক্ষরের বেশি হতে পারবে না", EN: "Last name can't be longer than 30 characters"},
	"firstMissing":  {BN: "প্রথম নাম দিন", EN: "Enter your first name"},
	"firstShort":    {BN: "প্রথম নাম কমপক্ষে ২ অক্ষরের হতে হবে", EN: "First name must be at least 2 characters"},
	"lastMissing":   {BN: "শেষ নাম দিন", EN: "Enter your last name"},
	"emailBad":      {BN: "সঠিক ইমেইল দিন", EN: "Enter a valid email"},
	"phoneBad":      {BN: "সঠিক মোবাইল নম্বর দিন (যেমন 01712345678)", EN: "Enter a valid mobile number (e.g. 01712345678)"},
	"passwordWeak":  {BN: "পাসওয়ার্ড কমপক্ষে ৮ অক্ষরের এবং অক্ষর ও সংখ্যা মিলিয়ে হতে হবে", EN: "Password must be at least 8 characters with letters and numbers"},
	"termsMissing":  {BN: "শর্তাবলীতে সম্মতি দিন", EN: "Please accept the terms"},
	"passwordWrong": {BN: "পাসওয়ার্ড মেলেনি", EN: "The password is incorrect"},
	"passwordSame":  {BN: "নতুন পাসওয়ার্ড পুরোনোটার মতো হতে পারবে না", EN: "The new password can't be the same as the old one"},
	"phoneSame":     {BN: "এটাই আপনার এখনকার নম্বর", EN: "This is already your number"},
	"required":      {BN: "এই ঘরটা পূরণ করুন", EN: "This field is required"},
}
