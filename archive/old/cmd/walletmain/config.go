package walletmain

import (
	"time"
	
	"github.com/urfave/cli"
)

// Config is the main configuration for wallet
type Config struct {
	// General application behavior
	ConfigFile    *string `short:"C" long:"configfile" description:"Path to configuration file"`
	ShowVersion   *bool   `short:"V" long:"version" description:"Display version information and exit"`
	LogLevel      *string
	Create        *bool   `long:"create" description:"Create the wallet if it does not exist"`
	CreateTemp    *bool   `long:"createtemp" description:"Create a temporary simulation wallet (pass=password) in the data directory indicated; must call with --datadir"`
	AppDataDir    *string `short:"A" long:"appdata" description:"Application data directory for wallet config, databases and logs"`
	TestNet3      *bool   `long:"testnet" description:"Use the test Bitcoin network (version 3) (default mainnet)"`
	SimNet        *bool   `long:"simnet" description:"Use the simulation test network (default mainnet)"`
	NoInitialLoad *bool   `long:"noinitialload" description:"Defer wallet creation/opening on startup and enable loading wallets over RPC"`
	LogDir        *string `long:"logdir" description:"Directory to log output."`
	Profile       *string `long:"profile" description:"Enable HTTP profiling on given port -- NOTE port must be between 1024 and 65536"`
	// GUI           *bool   `long:"__OLDgui" description:"Launch GUI"`
	// Wallet options
	WalletPass *string `long:"walletpass" default-mask:"-" description:"The public wallet password -- Only required if the wallet was created with one"`
	// RPC client options
	RPCConnect      *string `short:"c" long:"rpcconnect" description:"Hostname/IP and port of pod RPC server to connect to (default localhost:11048, testnet: localhost:21048, simnet: localhost:41048)"`
	CAFile          *string `long:"cafile" description:"File containing root certificates to authenticate a TLS connections with pod"`
	EnableClientTLS *bool   `long:"clienttls" description:"Enable TLS for the RPC client"`
	PodUsername     *string `long:"podusername" description:"Username for pod authentication"`
	PodPassword     *string `long:"podpassword" default-mask:"-" description:"Password for pod authentication"`
	Proxy           *string `long:"proxy" description:"Connect via SOCKS5 proxy (eg. 127.0.0.1:9050)"`
	ProxyUser       *string `long:"proxyuser" description:"Username for proxy server"`
	ProxyPass       *string `long:"proxypass" default-mask:"-" description:"Password for proxy server"`
	// SPV client options
	UseSPV       *bool            `long:"usespv" description:"Enables the experimental use of SPV rather than RPC for chain synchronization"`
	AddPeers     *cli.StringSlice `short:"a" long:"addpeer" description:"Add a peer to connect with at startup"`
	ConnectPeers *cli.StringSlice `long:"connect" description:"Connect only to the specified peers at startup"`
	MaxPeers     *int             `long:"maxpeers" description:"Max number of inbound and outbound peers"`
	BanDuration  *time.Duration   `long:"banduration" description:"How long to ban misbehaving peers.  Valid time units are {s, m, h}.  Minimum 1 second"`
	BanThreshold *int             `long:"banthreshold" description:"Maximum allowed ban score before disconnecting and banning misbehaving peers."`
	// RPC server options
	//
	// The legacy server is still enabled by default (and eventually will be replaced with the experimental server) so
	// prepare for that change by renaming the struct fields (but not the configuration options).
	//
	// Usernames can also be used for the consensus RPC client, so they aren't considered legacy.
	RPCCert                *string          `long:"rpccert" description:"File containing the certificate file"`
	RPCKey                 *string          `long:"rpckey" description:"File containing the certificate key"`
	OneTimeTLSKey          *bool            `long:"onetimetlskey" description:"Generate a new TLS certpair at startup, but only write the certificate to disk"`
	EnableServerTLS        *bool            `long:"servertls" description:"Enable TLS for the RPC server"`
	LegacyRPCListeners     *cli.StringSlice `long:"rpclisten" description:"Listen for legacy RPC connections on this interface/port (default port: 11046, testnet: 21046, simnet: 41046)"`
	LegacyRPCMaxClients    *int             `long:"rpcmaxclients" description:"Max number of legacy RPC clients for standard connections"`
	LegacyRPCMaxWebsockets *int             `long:"rpcmaxwebsockets" description:"Max number of legacy RPC websocket connections"`
	Username               *string          `short:"u" long:"username" description:"Username for legacy RPC and pod authentication (if podusername is unset)"`
	Password               *string          `short:"P" long:"password" default-mask:"-" description:"Password for legacy RPC and pod authentication (if podpassword is unset)"`
	// EXPERIMENTAL RPC server options
	//
	// These options will change (and require changes to config files, etc.) when the new gRPC server is enabled.
	ExperimentalRPCListeners *cli.StringSlice `long:"experimentalrpclisten" description:"Listen for RPC connections on this interface/port"`
	// Deprecated options
	DataDir *string `short:"b" long:"datadir" default-mask:"-" description:"DEPRECATED -- use appdata instead"`
}

// A bunch of constants
const (
	DefaultAppDataDirname   = "wallet"
	DefaultCAFilename       = "wallet.cert"
	DefaultConfigFilename   = "conf.json"
	DefaultLogLevel         = "info"
	DefaultLogDirname       = ""
	DefaultLogFilename      = "wallet/log"
	DefaultRPCMaxClients    = 10
	DefaultRPCMaxWebsockets = 25
	WalletDbName            = "wallet.db"
)

/*
// cleanAndExpandPath expands environement variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
// NOTE: The os.ExpandEnv doesn't work with Windows cmd.exe-style
	// %VARIABLE%, but they variables can still be expanded via POSIX-style
	// $VARIABLE.
	path = os.ExpandEnv(path)
	if !strings.HasPrefix(path, "~") {
return filepath.Clean(path)
	}
	// Expand initial ~ to the current user's home directory, or ~otheruser to otheruser's home directory.  On Windows, both forward and backward slashes can be used.
	path = path[1:]
	var pathSeparators string
	if runtime.GOOS == "windows" {
pathSeparators = string(os.PathSeparator) + "/"
	} else {
pathSeparators = string(os.PathSeparator)
	}
	userName := ""
	if i := strings.IndexAny(path, pathSeparators); i != -1 {
userName = path[:i]
		path = path[i:]
	}
	homeDir := ""
	var u *user.User
	var e error
	if userName == "" {
u, e = user.Current()
	} else {
u, e = user.Lookup(userName)
	}
	if e ==  nil {
homeDir = u.HomeDir
	}
	// Fallback to CWD if user lookup fails or user has no home directory.
	if homeDir == "" {
homeDir = "."
	}
	return filepath.Join(homeDir, path)
}
// createDefaultConfig creates a basic config file at the given destination path.
// For this it tries to read the config file for the RPC server (either pod or
// sac), and extract the RPC user and password from it.
func createDefaultConfigFile(destinationPath, serverConfigPath,
	serverDataDir, walletDataDir string) (e error) {
// fmt.Println("server config path", serverConfigPath)
	// Read the RPC server config
	serverConfigFile, e := os.Open(serverConfigPath)
	if e != nil  {
return e
	}
	defer serverConfigFile.Close()
	content, e := ioutil.ReadAll(serverConfigFile)
	if e != nil  {
return e
	}
	// content := []byte(samplePodCtlConf)
	// Extract the rpcuser
	rpcUserRegexp, e := regexp.Compile(`(?m)^\s*rpcuser=([^\s]+)`)
	if e != nil  {
return e
	}
	userSubmatches := rpcUserRegexp.FindSubmatch(content)
	if userSubmatches == nil {
// No user found, nothing to do
		return nil
	}
	// Extract the rpcpass
	rpcPassRegexp, e := regexp.Compile(`(?m)^\s*rpcpass=([^\s]+)`)
	if e != nil  {
return e
	}
	passSubmatches := rpcPassRegexp.FindSubmatch(content)
	if passSubmatches == nil {
// No password found, nothing to do
		return nil
	}
	// Extract the TLS
	TLSRegexp, e := regexp.Compile(`(?m)^\s*tls=(0|1)(?:\s|$)`)
	if e != nil  {
return e
	}
	TLSSubmatches := TLSRegexp.FindSubmatch(content)
	// Create the destination directory if it does not exists
	e = os.MkdirAll(filepath.Dir(destinationPath), 0700)
	if e != nil  {
return e
	}
	// fmt.Println("config path", destinationPath)
	// Create the destination file and write the rpcuser and rpcpass to it
	dest, e := os.OpenFile(destinationPath,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if e != nil  {
		return e
	}
	defer dest.Close()
	destString := fmt.Sprintf("username=%s\npassword=%s\n",
		string(userSubmatches[1]), string(passSubmatches[1]))
	if TLSSubmatches != nil {
	fmt.Println("TLS is enabled but more than likely the certificates will
fail verification because of the CA.
Currently there is no adequate tool for this, but will be soon.")
		destString += fmt.Sprintf("clienttls=%s\n", TLSSubmatches[1])
	}
	output := ";;; Defaults created from local pod/sac configuration:\n" + destString + "\n" + string(sampleModConf)
	dest.WriteString(output)
	return nil
}
func copy(src, dst string) (int64, error) {
// fmt.Println(src, dst)
	sourceFileStat, e := os.Stat(src)
	if e != nil  {
return 0, e
	}
	if !sourceFileStat.Mode().IsRegular() {
return 0, fmt.Errorf("%s is not a regular file", src)
	}
	source, e := os.Open(src)
	if e != nil  {
return 0, e
	}
	defer source.Close()
	destination, e := os.Create(dst)
	if e != nil  {
return 0, e
	}
	defer destination.Close()
	nBytes, e := io.Copy(destination, source)
	return nBytes, e
}
// supportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.
func supportedSubsystems() []string {
// Convert the subsystemLoggers map keys to a slice.
	subsystems := make([]string, 0, len(subsystemLoggers))
	for subsysID := range subsystemLoggers {
subsystems = append(subsystems, subsysID)
	}
	// Sort the subsytems for stable display.
	txsort.Strings(subsystems)
	return subsystems
}
// parseAndSetDebugLevels attempts to parse the specified debug level and set
// the levels accordingly.  An appropriate error is returned if anything is
// invalid.
func parseAndSetDebugLevels(debugLevel string) (e error) {
// When the specified string doesn't have any delimters, treat it as
	// the log level for all subsystems.
	if !strings.Contains(debugLevel, ",") && !strings.Contains(debugLevel, "=") {
// Validate debug log level.
	if !validLogLevel(debugLevel) {
str := "The specified debug level [%v] is invalid"
		return fmt.Errorf(str, debugLevel)
	}
	// Change the logging level for all subsystems.
	setLogLevels(debugLevel)
	return nil
}
// Split the specified string into subsystem/level pairs while detecting
// issues and update the log levels accordingly.
for _, logLevelPair := range strings.Split(debugLevel, ",") {
if !strings.Contains(logLevelPair, "=") {
str := "The specified debug level contains an invalid " +
		"subsystem/level pair [%v]"
		return fmt.Errorf(str, logLevelPair)
	}
	// Extract the specified subsystem and log level.
	fields := strings.Split(logLevelPair, "=")
	subsysID, logLevel := fields[0], fields[1]
	// Validate subsystem.
	if _, exists := subsystemLoggers[subsysID]; !exists {
str := "The specified subsystem [%v] is invalid -- " +
		"supported subsytems %v"
		return fmt.Errorf(str, subsysID, supportedSubsystems())
	}
	// Validate log level.
	if !validLogLevel(logLevel) {
str := "The specified debug level [%v] is invalid"
		return fmt.Errorf(str, logLevel)
	}
	setLogLevel(subsysID, logLevel)
	}
	return nil
}
// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
//      1) Start with a default config with sane settings
//      2) Pre-parse the command line to check for an alternative config file
//      3) Load configuration file overwriting defaults with any specified options
//      4) Parse CLI options and overwrite/add any specified options
//
// The above results in btcwallet functioning properly without any config
// settings while still allowing the user to override settings with config files
// and command line options.  Command line options always take precedence.
func loadConfig(	cfg *Config) (*Config, []string, error) {
cfg = Config{
				ConfigFile:             DefaultConfigFile,
				AppDataDir:             DefaultAppDataDir,
				LogDir:                 DefaultLogDir,
				WalletPass:             wallet.InsecurePubPassphrase,
				CAFile:                 "",
				RPCKey:                 DefaultRPCKeyFile,
				RPCCert:                DefaultRPCCertFile,
				WalletRPCMaxClients:    DefaultRPCMaxClients,
				WalletRPCMaxWebsockets: DefaultRPCMaxWebsockets,
				DataDir:                DefaultAppDataDir,
			// AddPeers:               []string{},
			// ConnectPeers:           []string{},
		}
		// Pre-parse the command line options to see if an alternative config
		// file or the version flag was specified.
		preCfg := cfg
		preParser := flags.NewParser(&preCfg, flags.Default)
		_, e := preParser.Parse()
		if e != nil  {
if e, ok := e.(*flags.Error); !ok || e.Type != flags.ErrHelp {
preParser.WriteHelp(os.Stderr)
					}
			return nil, nil, e
		}
		// Show the version and exit if the version flag was specified.
		funcName := "loadConfig"
		appName := filepath.Base(os.Args[0])
		appName = strings.TrimSuffix(appName, filepath.Ext(appName))
		usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
		if preCfg.ShowVersion {
fmt.Println(appName, "version", version())
				os.Exit(0)
		}
		// Load additional config from file.
		var configFileError error
		parser := flags.NewParser(&cfg, flags.Default)
		configFilePath := preCfg.ConfigFile.value
		if preCfg.ConfigFile.ExplicitlySet() {
configFilePath = cleanAndExpandPath(configFilePath)
			} else {
appDataDir := preCfg.AppDataDir.value
					if !preCfg.AppDataDir.ExplicitlySet() && preCfg.DataDir.ExplicitlySet() {
appDataDir = cleanAndExpandPath(preCfg.DataDir.value)
						}
						if appDataDir != DefaultAppDataDir {
configFilePath = filepath.Join(appDataDir, DefaultConfigFilename)
							}
						}
		e = flags.NewIniParser(parser).ParseFile(configFilePath)
		if e != nil  {
if _, ok := e.(*os.PathError); !ok {
fmt.Fprintln(os.Stderr, e)
				parser.WriteHelp(os.Stderr)
				return nil, nil, e
			}
			configFileError = e
		}
		// Parse command line options again to ensure they take precedence.
		remainingArgs, e := parser.Parse()
		if e != nil  {
if e, ok := e.(*flags.Error); !ok || e.Type != flags.ErrHelp {
parser.WriteHelp(os.Stderr)
			}
		return nil, nil, e
		}
		// Chk deprecated aliases.  The new options receive priority when both
		// are changed from the default.
		if cfg.DataDir.ExplicitlySet() {
fmt.Fprintln(os.Stderr, "datadir option has been replaced by "+
					"appdata -- please update your config")
				if !cfg.AppDataDir.ExplicitlySet() {
cfg.AppDataDir.value = cfg.DataDir.value
					}
				}
				// If an alternate data directory was specified, and paths with defaults
				// relative to the data dir are unchanged, modify each path to be
				// relative to the new data dir.
				if cfg.AppDataDir.ExplicitlySet() {
cfg.AppDataDir.value = cleanAndExpandPath(cfg.AppDataDir.value)
						if !cfg.RPCKey.ExplicitlySet() {
cfg.RPCKey.value = filepath.Join(cfg.AppDataDir.value, "rpc.key")
							}
							if !cfg.RPCCert.ExplicitlySet() {
cfg.RPCCert.value = filepath.Join(cfg.AppDataDir.value, "rpc.cert")
								}
							}
							if _, e = os.Stat(cfg.DataDir.value); os.IsNotExist(e) {
// Create the destination directory if it does not exists
									e = os.MkdirAll(cfg.DataDir.value, 0700)
									if e != nil  {
fmt.Println("ERROR", e)
											return nil, nil, e
										}
									}
									var generatedRPCPass, generatedRPCUser string
		if _, e = os.Stat(cfg.ConfigFile.value); os.IsNotExist(e) {
// If we can find a pod.conf in the standard location, copy
				// copy the rpcuser and rpcpassword and TLS setting
				c := cleanAndExpandPath("~/.pod/pod.conf")
				// fmt.Println("server config path:", c)
			// _, e = os.Stat(c)
			// fmt.Println(e)
			// fmt.Println(os.IsNotExist(err))
			if _, e = os.Stat(c); e ==  nil {
fmt.Println("Creating config from pod config")
					createDefaultConfigFile(cfg.ConfigFile.value, c, cleanAndExpandPath("~/.pod"),
						cfg.AppDataDir.value)
				} else {
var bb bytes.Buffer
						bb.Write(sampleModConf)
				fmt.Println("Writing config file:", cfg.ConfigFile.value)
				dest, e := os.OpenFile(cfg.ConfigFile.value,
						os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
					if e != nil  {
fmt.Println("ERROR", e)
							return nil, nil, e
						}
						defer dest.Close()
						// We generate a random user and password
						randomBytes := make([]byte, 20)
				_, e = rand.Read(randomBytes)
				if e != nil  {
return nil, nil, e
					}
					generatedRPCUser = base64.StdEncoding.EncodeToString(randomBytes)
					_, e = rand.Read(randomBytes)
				if e != nil  {
return nil, nil, e
					}
				generatedRPCPass = base64.StdEncoding.EncodeToString(randomBytes)
				// We copy every line from the sample config file to the destination,
				// only replacing the two lines for rpcuser and rpcpass
				//
				var line string
				reader := bufio.NewReader(&bb)
				for e != io.EOF {
line, e = reader.ReadString('\n')
						if e != nil  && e != io.EOF {
return nil, nil, e
							}
							if !strings.Contains(line, "podusername=") && !strings.Contains(line, "podpassword=") {
if strings.Contains(line, "username=") {
line = "username=" + generatedRPCUser + "\n"
										} else if strings.Contains(line, "password=") {
line = "password=" + generatedRPCPass + "\n"
											}
										}
										_, _ = generatedRPCPass, generatedRPCUser
										if _, e = dest.WriteString(line); E.Chk(e) {
return nil, nil, e
											}
										}
									}
								}
								// Choose the active network netparams based on the selected network.
								// Multiple networks can't be selected simultaneously.
								numNets := 0
								if cfg.TestNet3 {
activeNet = &chaincfg.TestNet3Params
										numNets++
									}
									if cfg.SimNet {
activeNet = &chaincfg.SimNetParams
			numNets++
		}
		if numNets > 1 {
str := "%s: The testnet and simnet netparams can't be used " +
				"together -- choose one"
			e := fmt.Errorf(str, "loadConfig")
			fmt.Fprintln(os.Stderr, e)
			parser.WriteHelp(os.Stderr)
			return nil, nil, e
		}
		// Append the network type to the log directory so it is "namespaced"
		// per network.
		cfg.LogDir = cleanAndExpandPath(cfg.LogDir)
		cfg.LogDir = filepath.Join(cfg.LogDir, activeNet.Params.Name)
		// Special show command to list supported subsystems and exit.
		if cfg.DebugLevel == "show" {
fmt.Println("Supported subsystems", supportedSubsystems())
				os.Exit(0)
			}
			// Initialize log rotation.  After log rotation has been initialized, the
			// logger variables may be used.
			initLogRotator(filepath.Join(cfg.LogDir, DefaultLogFilename))
			// Parse, validate, and set debug log level(s).
			if e := parseAndSetDebugLevels(cfg.DebugLevel); E.Chk(e) {
e := fmt.Errorf("%s: %v", "loadConfig", e.Error())
			fmt.Fprintln(os.Stderr, e)
			parser.WriteHelp(os.Stderr)
			return nil, nil, e
		}
		// Exit if you try to use a simulation wallet with a standard
		// data directory.
		if !(cfg.AppDataDir.ExplicitlySet() || cfg.DataDir.ExplicitlySet()) && cfg.CreateTemp {
fmt.Fprintln(os.Stderr, "Tried to create a temporary simulation "+
					"wallet, but failed to specify data directory!")
				os.Exit(0)
			}
			// Exit if you try to use a simulation wallet on anything other than
			// simnet or testnet3.
		if !cfg.SimNet && cfg.CreateTemp {
fmt.Fprintln(os.Stderr, "Tried to create a temporary simulation "+
				"wallet for network other than simnet!")
			os.Exit(0)
		}
		// Ensure the wallet exists or create it when the create flag is set.
		netDir := NetworkDir(cfg.AppDataDir, ActiveNet.Params)
		dbPath := filepath.Join(netDir, WalletDbName)
		if cfg.CreateTemp && cfg.Create {
e := fmt.Errorf("The flags --create and --createtemp can not " +
					"be specified together. Use --help for more information.")
				fmt.Fprintln(os.Stderr, e)
				return nil, nil, e
			}
			dbFileExists, e := cfgutil.FileExists(dbPath)
			if e != nil  {
log <- cl.Error{err}
					return nil, nil, e
				}
				if cfg.CreateTemp {
tempWalletExists := false
						if dbFileExists {
str := fmt.Sprintf("The wallet already exists. Loading this " +
									"wallet instead.")
								fmt.Fprintln(os.Out, str)
								tempWalletExists = true
							}
							// Ensure the data directory for the network exists.
							if e := checkCreateDir(netDir); E.Chk(e) {
fmt.Fprintln(os.Stderr, e)
									return nil, nil, e
								}
								if !tempWalletExists {
// Perform the initial wallet creation wizard.
										if e := createSimulationWallet(&cfg); E.Chk(e) {
fmt.Fprintln(os.Stderr, "Unable to create wallet:", e)
												return nil, nil, e
											}
										}
		} else if cfg.Create {
// Error if the create flag is set and the wallet already
			// exists.
			if dbFileExists {
e := fmt.Errorf("The wallet database file `%v` "+
						"already exists.", dbPath)
					fmt.Fprintln(os.Stderr, e)
					return nil, nil, e
			}
			// Ensure the data directory for the network exists.
			if e := checkCreateDir(netDir); E.Chk(e) {
fmt.Fprintln(os.Stderr, e)
				return nil, nil, e
			}
			// Perform the initial wallet creation wizard.
			if e := createWallet(&cfg); E.Chk(e) {
fmt.Fprintln(os.Stderr, "Unable to create wallet:", e)
					return nil, nil, e
				}
				// Created successfully, so exit now with success.
				os.Exit(0)
			} else if !dbFileExists && !cfg.NoInitialLoad {
keystorePath := filepath.Join(netDir, keystore.Filename)
					keystoreExists, e := cfgutil.FileExists(keystorePath)
					if e != nil  {
fmt.Fprintln(os.Stderr, e)
							return nil, nil, e
			}
			if !keystoreExists {
// e = fmt.Errorf("The wallet does not exist.  Run with the " +
				// "--create option to initialize and create it...")
				// Ensure the data directory for the network exists.
				fmt.Println("Existing wallet not found in", cfg.ConfigFile.value)
				if e := checkCreateDir(netDir); E.Chk(e) {
fmt.Fprintln(os.Stderr, e)
						return nil, nil, e
					}
					// Perform the initial wallet creation wizard.
					if e := createWallet(&cfg); E.Chk(e) {
fmt.Fprintln(os.Stderr, "Unable to create wallet:", e)
					return nil, nil, e
				}
				// Created successfully, so exit now with success.
				os.Exit(0)
			} else {
e = fmt.Errorf("The wallet is in legacy format.  Run with the " +
						"--create option to import it.")
				}
				fmt.Fprintln(os.Stderr, e)
				return nil, nil, e
			}
			// localhostListeners := map[string]struct{}{
				// 	"localhost": {},
		// 	"127.0.0.1": {},
		// 	"::1":       {},
		// }
		// if cfg.UseSPV {
// 	sac.MaxPeers = cfg.MaxPeers
			// 	sac.BanDuration = cfg.BanDuration
			// 	sac.BanThreshold = cfg.BanThreshold
			// } else {
if cfg.RPCConnect == "" {
cfg.RPCConnect = net.JoinHostPort("localhost", activeNet.RPCClientPort)
					}
					// Add default port to connect flag if missing.
					cfg.RPCConnect, e = cfgutil.NormalizeAddress(cfg.RPCConnect,
							activeNet.RPCClientPort)
						if e != nil  {
fmt.Fprintf(os.Stderr,
										"Invalid rpcconnect network address: %v\n", e)
									return nil, nil, e
								}
		// RPCHost, _, e = net.SplitHostPort(cfg.RPCConnect)
		// if e != nil  {
// 	return nil, nil, e
		// }
		if cfg.EnableClientTLS {
// if _, ok := localhostListeners[RPCHost]; !ok {
// 	str := "%s: the --noclienttls option may not be used " +
					// 		"when connecting RPC to non localhost " +
					// 		"addresses: %s"
					// 	e := fmt.Errorf(str, funcName, cfg.RPCConnect)
					// 	fmt.Fprintln(os.Stderr, e)
					// 	fmt.Fprintln(os.Stderr, usageMessage)
					// 	return nil, nil, e
					// }
					// } else {
// If CAFile is unset, choose either the copy or local pod cert.
						if !cfg.CAFile.ExplicitlySet() {
cfg.CAFile.value = filepath.Join(cfg.AppDataDir.value, DefaultCAFilename)
				// If the CA copy does not exist, check if we're connecting to
				// a local pod and switch to its RPC cert if it exists.
				certExists, e := cfgutil.FileExists(cfg.CAFile.value)
				if e != nil  {
fmt.Fprintln(os.Stderr, e)
						return nil, nil, e
				}
				if !certExists {
// if _, ok := localhostListeners[RPCHost]; ok {
podCertExists, e := cfgutil.FileExists(
									DefaultCAFile)
								if e != nil  {
fmt.Fprintln(os.Stderr, e)
										return nil, nil, e
					}
					if podCertExists {
cfg.CAFile.value = DefaultCAFile
						}
						// }
					}
				}
			}
			// }
			// Only set default RPC listeners when there are no listeners set for
			// the experimental RPC server.  This is required to prevent the old RPC
			// server from sharing listen addresses, since it is impossible to
			// remove defaults from go-flags slice options without assigning
			// specific behavior to a particular string.
			if len(cfg.ExperimentalRPCListeners) == 0 && len(cfg.WalletRPCListeners) == 0 {
addrs, e := net.LookupHost("localhost")
			if e != nil  {
return nil, nil, e
				}
				cfg.WalletRPCListeners = make([]string, 0, len(addrs))
				for _, addr := range addrs {
addr = net.JoinHostPort(addr, activeNet.WalletRPCServerPort)
						cfg.WalletRPCListeners = append(cfg.WalletRPCListeners, addr)
					}
				}
				// Add default port to all rpc listener addresses if needed and remove
				// duplicate addresses.
		cfg.WalletRPCListeners, e = cfgutil.NormalizeAddresses(
				cfg.WalletRPCListeners, activeNet.WalletRPCServerPort)
			if e != nil  {
fmt.Fprintf(os.Stderr,
							"Invalid network address in legacy RPC listeners: %v\n", e)
						return nil, nil, e
					}
					cfg.ExperimentalRPCListeners, e = cfgutil.NormalizeAddresses(
			cfg.ExperimentalRPCListeners, activeNet.WalletRPCServerPort)
		if e != nil  {
fmt.Fprintf(os.Stderr,
						"Invalid network address in RPC listeners: %v\n", e)
					return nil, nil, e
				}
				// Both RPC servers may not listen on the same interface/port.
				if len(cfg.WalletRPCListeners) > 0 && len(cfg.ExperimentalRPCListeners) > 0 {
seenAddresses := make(map[string]struct{}, len(cfg.WalletRPCListeners))
			for _, addr := range cfg.WalletRPCListeners {
seenAddresses[addr] = struct{}{}
			}
			for _, addr := range cfg.ExperimentalRPCListeners {
_, seen := seenAddresses[addr]
					if seen {
e := fmt.Errorf("Address `%s` may not be "+
						"used as a listener address for both "+
						"RPC servers", addr)
					fmt.Fprintln(os.Stderr, e)
					return nil, nil, e
				}
			}
		}
		// Only allow server TLS to be disabled if the RPC server is bound to
		// localhost addresses.
		if !cfg.EnableServerTLS {
allListeners := append(cfg.WalletRPCListeners,
						cfg.ExperimentalRPCListeners...)
					for _, addr := range allListeners {
if e != nil  {
str := "%s: RPC listen interface '%s' is " +
										"invalid: %v"
									e := fmt.Errorf(str, funcName, addr, e)
									fmt.Fprintln(os.Stderr, e)
									fmt.Fprintln(os.Stderr, usageMessage)
									return nil, nil, e
								}
				// host, _, e = net.SplitHostPort(addr)
				// if _, ok := localhostListeners[host]; !ok {
// 	str := "%s: the --noservertls option may not be used " +
					// 		"when binding RPC to non localhost " +
				// 		"addresses: %s"
				// 	e := fmt.Errorf(str, funcName, addr)
				// 	fmt.Fprintln(os.Stderr, e)
				// 	fmt.Fprintln(os.Stderr, usageMessage)
				// 	return nil, nil, e
				// }
			}
		}
		// Expand environment variable and leading ~ for filepaths.
		cfg.CAFile.value = cleanAndExpandPath(cfg.CAFile.value)
		cfg.RPCCert.value = cleanAndExpandPath(cfg.RPCCert.value)
		cfg.RPCKey.value = cleanAndExpandPath(cfg.RPCKey.value)
		// If the pod username or password are unset, use the same auth as for
		// the client.  The two settings were previously shared for pod and
		// client auth, so this avoids breaking backwards compatibility while
		// allowing users to use different auth settings for pod and wallet.
		if cfg.PodUsername == "" {
cfg.PodUsername = cfg.Username
			}
		if cfg.PodPassword == "" {
cfg.PodPassword = cfg.Password
			}
			// Warn about missing config file after the final command line parse
			// succeeds.  This prevents the warning on help messages and invalid
			// options.
			if configFileError != nil {
Log.Warnf.Print("%v", configFileError)
				}
				return cfg, nil, nil
			}
			// validLogLevel returns whether or not logLevel is a valid debug log level.
			func validLogLevel(				logLevel string) bool {
switch logLevel {
case "trace":
						fallthrough
					case "debug":
						fallthrough
					case "info":
						fallthrough
					case "warn":
						fallthrough
					case "error":
						fallthrough
					case "critical":
						return true
					}
					return false
				}
*/
