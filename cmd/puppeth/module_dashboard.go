















package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/log"
)



var dashboardContent = `
<!DOCTYPE html>
<html lang="en">
	<head>
		<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
		<!-- Meta, title, CSS, favicons, etc. -->
		<meta charset="utf-8">
		<meta http-equiv="X-UA-Compatible" content="IE=edge">
		<meta name="viewport" content="width=device-width, initial-scale=1">

		<title>{{.NetworkTitle}}: Network Dashboard</title>

		<link href="https:
		<link href="https:
		<link href="https:
		<style>
			.vertical-center {
				min-height: 100%;
				min-height: 95vh;
				display: flex;
				align-items: center;
			}
			.nav.side-menu li a {
				font-size: 18px;
			}
			.nav-sm .nav.side-menu li a {
				font-size: 10px;
			}
			pre{
				white-space: pre-wrap;
			}
		</style>
	</head>

	<body class="nav-sm" style="overflow-x: hidden">
		<div class="container body">
			<div class="main_container">
				<div class="col-md-3 left_col">
					<div class="left_col scroll-view">
						<div class="navbar nav_title" style="border: 0; margin-top: 8px;">
							<a class="site_title"><i class="fa fa-globe" style="margin-left: 6px"></i> <span>{{.NetworkTitle}} Testnet</span></a>
						</div>
						<div class="clearfix"></div>
						<br />
						<div id="sidebar-menu" class="main_menu_side hidden-print main_menu">
							<div class="menu_section">
								<ul class="nav side-menu">
									{{if .EthstatsPage}}<li id="stats_menu"><a onclick="load('#stats')"><i class="fa fa-tachometer"></i> Network Stats</a></li>{{end}}
									{{if .ExplorerPage}}<li id="explorer_menu"><a onclick="load('#explorer')"><i class="fa fa-database"></i> Block Explorer</a></li>{{end}}
									{{if .WalletPage}}<li id="wallet_menu"><a onclick="load('#wallet')"><i class="fa fa-address-book-o"></i> Browser Wallet</a></li>{{end}}
									{{if .FaucetPage}}<li id="faucet_menu"><a onclick="load('#faucet')"><i class="fa fa-bath"></i> Crypto Faucet</a></li>{{end}}
									<li id="connect_menu"><a><i class="fa fa-plug"></i> Connect Yourself</a>
										<ul id="connect_list" class="nav child_menu">
											<li><a onclick="$('#connect_menu').removeClass('active'); $('#connect_list').toggle(); load('#geth')">Go Ethereum: Geth</a></li>
											<li><a onclick="$('#connect_menu').removeClass('active'); $('#connect_list').toggle(); load('#mist')">Go Ethereum: Wallet & Mist</a></li>
											<li><a onclick="$('#connect_menu').removeClass('active'); $('#connect_list').toggle(); load('#mobile')">Go Ethereum: Android & iOS</a></li>{{if .Ethash}}
											<li><a onclick="$('#connect_menu').removeClass('active'); $('#connect_list').toggle(); load('#other')">Other Ethereum Clients</a></li>{{end}}
										</ul>
									</li>
									<li id="about_menu"><a onclick="load('#about')"><i class="fa fa-heartbeat"></i> About Puppeth</a></li>
								</ul>
							</div>
						</div>
					</div>
				</div>
				<div class="right_col" role="main" style="padding: 0 !important">
					<div id="geth" hidden style="padding: 16px;">
						<div class="page-title">
							<div class="title_left">
								<h3>Connect Yourself &ndash; Go Ethereum: Geth</h3>
							</div>
						</div>
						<div class="clearfix"></div>
						<div class="row">
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2><i class="fa fa-archive" aria-hidden="true"></i> Archive node <small>Retains all historical data</small></h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>An archive node synchronizes the blockchain by downloading the full chain from the genesis block to the current head block, executing all the transactions contained within. As the node crunches through the transactions, all past historical state is stored on disk, and can be queried for each and every block.</p>
										<p>Initial processing required to execute all transactions may require non-negligible time and disk capacity required to store all past state may be non-insignificant. High end machines with SSD storage, modern CPUs and 8GB+ RAM are recommended.</p>
										<br/>
										<p>To run an archive node, download <a href="/{{.GethGenesis}}"><code>{{.GethGenesis}}</code></a> and start Geth with:
											<pre>geth --datadir=$HOME/.{{.Network}} init {{.GethGenesis}}</pre>
											<pre>geth --networkid={{.NetworkID}} --datadir=$HOME/.{{.Network}} --cache=1024 --syncmode=full{{if .Ethstats}} --ethstats='{{.Ethstats}}'{{end}} --bootnodes={{.BootnodesFlat}}</pre>
										</p>
										<br/>
										<p>You can download Geth from <a href="https:
									</div>
								</div>
							</div>
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2><i class="fa fa-laptop" aria-hidden="true"></i> Full node <small>Retains recent data only</small></h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>A full node synchronizes the blockchain by downloading the full chain from the genesis block to the current head block, but does not execute the transactions. Instead, it downloads all the transactions receipts along with the entire recent state. As the node downloads the recent state directly, historical data can only be queried from that block onward.</p>
										<p>Initial processing required to synchronize is more bandwidth intensive, but is light on the CPU and has significantly reduced disk requirements. Mid range machines with HDD storage, decent CPUs and 4GB+ RAM should be enough.</p>
										<br/>
										<p>To run a full node, download <a href="/{{.GethGenesis}}"><code>{{.GethGenesis}}</code></a> and start Geth with:
											<pre>geth --datadir=$HOME/.{{.Network}} init {{.GethGenesis}}</pre>
											<pre>geth --networkid={{.NetworkID}} --datadir=$HOME/.{{.Network}} --cache=512{{if .Ethstats}} --ethstats='{{.Ethstats}}'{{end}} --bootnodes={{.BootnodesFlat}}</pre>
										</p>
										<br/>
										<p>You can download Geth from <a href="https:
									</div>
								</div>
							</div>
						</div>
						<div class="clearfix"></div>
						<div class="row">
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2><i class="fa fa-mobile" aria-hidden="true"></i> Light node <small>Retrieves data on demand</small></h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>A light node synchronizes the blockchain by downloading and verifying only the chain of headers from the genesis block to the current head, without executing any transactions or retrieving any associated state. As no state is available locally, any interaction with the blockchain relies on on-demand data retrievals from remote nodes.</p>
										<p>Initial processing required to synchronize is light, as it only verifies the validity of the headers; similarly required disk capacity is small, tallying around 500 bytes per header. Low end machines with arbitrary storage, weak CPUs and 512MB+ RAM should cope well.</p>
										<br/>
										<p>To run a light node, download <a href="/{{.GethGenesis}}"><code>{{.GethGenesis}}</code></a> and start Geth with:
											<pre>geth --datadir=$HOME/.{{.Network}} init {{.GethGenesis}}</pre>
											<pre>geth --networkid={{.NetworkID}} --datadir=$HOME/.{{.Network}} --syncmode=light{{if .Ethstats}} --ethstats='{{.Ethstats}}'{{end}} --bootnodes={{.BootnodesFlat}}</pre>
										</p>
										<br/>
										<p>You can download Geth from <a href="https:
									</div>
								</div>
							</div>
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2><i class="fa fa-microchip" aria-hidden="true"></i> Embedded node <small>Conserves memory vs. speed</small></h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>An embedded node is a variation of the light node with configuration parameters tuned towards low memory footprint. As such, it may sacrifice processing and disk IO performance to conserve memory. It should be considered an <strong>experimental</strong> direction for now without hard guarantees or bounds on the resources used.</p>
										<p>Initial processing required to synchronize is light, as it only verifies the validity of the headers; similarly required disk capacity is small, tallying around 500 bytes per header. Embedded machines with arbitrary storage, low power CPUs and 128MB+ RAM may work.</p>
										<br/>
										<p>To run an embedded node, download <a href="/{{.GethGenesis}}"><code>{{.GethGenesis}}</code></a> and start Geth with:
											<pre>geth --datadir=$HOME/.{{.Network}} init {{.GethGenesis}}</pre>
											<pre>geth --networkid={{.NetworkID}} --datadir=$HOME/.{{.Network}} --cache=16 --ethash.cachesinmem=1 --syncmode=light{{if .Ethstats}} --ethstats='{{.Ethstats}}'{{end}} --bootnodes={{.BootnodesFlat}}</pre>
										</p>
										<br/>
										<p>You can download Geth from <a href="https:
									</div>
								</div>
							</div>
						</div>
					</div>
					<div id="mist" hidden style="padding: 16px;">
						<div class="page-title">
							<div class="title_left">
								<h3>Connect Yourself &ndash; Go Ethereum: Wallet &amp; Mist</h3>
							</div>
						</div>
						<div class="clearfix"></div>
						<div class="row">
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2><i class="fa fa-credit-card" aria-hidden="true"></i> Desktop wallet <small>Interacts with accounts and contracts</small></h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>The Ethereum Wallet is an <a href="https:
										<p>Under the hood the wallet is backed by a go-ethereum full node, meaning that a mid range machine is assumed. Similarly, synchronization is based on <strong>fast-sync</strong>, which will download all blockchain data from the network and make it available to the wallet. Light nodes cannot currently fully back the wallet, but it's a target actively pursued.</p>
										<br/>
										<p>To connect with the Ethereum Wallet, you'll need to initialize your private network first via Geth as the wallet does not currently support calling Geth directly. To initialize your local chain, download <a href="/{{.GethGenesis}}"><code>{{.GethGenesis}}</code></a> and run:
											<pre>geth --datadir=$HOME/.{{.Network}} init {{.GethGenesis}}</pre>
										</p>
										<p>With your local chain initialized, you can start the Ethereum Wallet:
											<pre>ethereumwallet --rpc $HOME/.{{.Network}}/geth.ipc --node-networkid={{.NetworkID}} --node-datadir=$HOME/.{{.Network}}{{if .Ethstats}} --node-ethstats='{{.Ethstats}}'{{end}} --node-bootnodes={{.BootnodesFlat}}</pre>
										<p>
										<br/>
										<p>You can download the Ethereum Wallet from <a href="https:
									</div>
								</div>
							</div>
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2><i class="fa fa-picture-o" aria-hidden="true"></i> Mist browser <small>Interacts with third party DApps</small></h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>The Mist browser is an <a href="https:
										<p>Under the hood the browser is backed by a go-ethereum full node, meaning that a mid range machine is assumed. Similarly, synchronization is based on <strong>fast-sync</strong>, which will download all blockchain data from the network and make it available to the wallet. Light nodes cannot currently fully back the wallet, but it's a target actively pursued.</p>
										<br/>
										<p>To connect with the Mist browser, you'll need to initialize your private network first via Geth as Mist does not currently support calling Geth directly. To initialize your local chain, download <a href="/{{.GethGenesis}}"><code>{{.GethGenesis}}</code></a> and run:
											<pre>geth --datadir=$HOME/.{{.Network}} init {{.GethGenesis}}</pre>
										</p>
										<p>With your local chain initialized, you can start Mist:
											<pre>mist --rpc $HOME/.{{.Network}}/geth.ipc --node-networkid={{.NetworkID}} --node-datadir=$HOME/.{{.Network}}{{if .Ethstats}} --node-ethstats='{{.Ethstats}}'{{end}} --node-bootnodes={{.BootnodesFlat}}</pre>
										<p>
										<br/>
										<p>You can download the Mist browser from <a href="https:
									</div>
								</div>
							</div>
						</div>
					</div>
					<div id="mobile" hidden style="padding: 16px;">
						<div class="page-title">
							<div class="title_left">
								<h3>Connect Yourself &ndash; Go Ethereum: Android &amp; iOS</h3>
							</div>
						</div>
						<div class="clearfix"></div>
						<div class="row">
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2><i class="fa fa-android" aria-hidden="true"></i> Android devices <small>Accesses Ethereum via Java</small></h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>Starting with the 1.5 release of go-ethereum, we've transitioned away from shipping only full blown Ethereum clients and started focusing on releasing the code as reusable packages initially for Go projects, then later for Java based Android projects too. Mobile support is still evolving, hence is bound to change often and hard, but the Ethereum network can nonetheless be accessed from Android too.</p>
										<p>Under the hood the Android library is backed by a go-ethereum light node, meaning that given a not-too-old Android device, you should be able to join the network without significant issues. Certain functionality is not yet available and rough edges are bound to appear here and there, please report issues if you find any.</p>
										<br/>
										<p>The stable Android archives are distributed via Maven Central, and the develop snapshots via the Sonatype repositories. Before proceeding, please ensure you have a recent version configured in your Android project. You can find details in <a href="https:
										<p>Before connecting to the Ethereum network, download the <a href="/{{.GethGenesis}}"><code>{{.GethGenesis}}</code></a> genesis json file and either store it in your Android project as a resource file you can access, or save it as a string in a variable. You're going to need to initialize your client.</p>
										<p>Inside your Java code you can now import the geth archive and connect to Ethereum:
											<pre>import org.ethereum.geth.*;</pre>
<pre>
Enodes bootnodes = new Enodes();{{range .Bootnodes}}
bootnodes.append(new Enode("{{.}}"));{{end}}

NodeConfig config = new NodeConfig();
config.setBootstrapNodes(bootnodes);
config.setEthereumNetworkID({{.NetworkID}});
config.setEthereumGenesis(genesis);{{if .Ethstats}}
config.setEthereumNetStats("{{.Ethstats}}");{{end}}

Node node = new Node(getFilesDir() + "/.{{.Network}}", config);
node.start();
</pre>
										<p>
									</div>
								</div>
							</div>
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2><i class="fa fa-apple" aria-hidden="true"></i> iOS devices <small>Accesses Ethereum via ObjC/Swift</small></h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>Starting with the 1.5 release of go-ethereum, we've transitioned away from shipping only full blown Ethereum clients and started focusing on releasing the code as reusable packages initially for Go projects, then later for ObjC/Swift based iOS projects too. Mobile support is still evolving, hence is bound to change often and hard, but the Ethereum network can nonetheless be accessed from iOS too.</p>
										<p>Under the hood the iOS library is backed by a go-ethereum light node, meaning that given a not-too-old Apple device, you should be able to join the network without significant issues. Certain functionality is not yet available and rough edges are bound to appear here and there, please report issues if you find any.</p>
										<br/>
										<p>Both stable and develop builds of the iOS framework are available via CocoaPods. Before proceeding, please ensure you have a recent version configured in your iOS project. You can find details in <a href="https:
										<p>Before connecting to the Ethereum network, download the <a href="/{{.GethGenesis}}"><code>{{.GethGenesis}}</code></a> genesis json file and either store it in your iOS project as a resource file you can access, or save it as a string in a variable. You're going to need to initialize your client.</p>
										<p>Inside your Swift code you can now import the geth framework and connect to Ethereum (ObjC should be analogous):
											<pre>import Geth</pre>
<pre>
var error: NSError?

let bootnodes = GethNewEnodesEmpty(){{range .Bootnodes}}
bootnodes?.append(GethNewEnode("{{.}}", &error)){{end}}

let config = GethNewNodeConfig()
config?.setBootstrapNodes(bootnodes)
config?.setEthereumNetworkID({{.NetworkID}})
config?.setEthereumGenesis(genesis){{if .Ethstats}}
config?.setEthereumNetStats("{{.Ethstats}}"){{end}}

let datadir = NSSearchPathForDirectoriesInDomains(.documentDirectory, .userDomainMask, true)[0]
let node = GethNewNode(datadir + "/.{{.Network}}", config, &error);
try! node?.start();
</pre>
										<p>
									</div>
								</div>
							</div>
						</div>
					</div>{{if .Ethash}}
					<div id="other" hidden style="padding: 16px;">
						<div class="page-title">
							<div class="title_left">
								<h3>Connect Yourself &ndash; Other Ethereum Clients</h3>
							</div>
						</div>
						<div class="clearfix"></div>
						<div class="row">
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2>
											<svg height="14px" xmlns="http:
											C++ Ethereum <small>Official C++ client from the Ethereum Foundation</small>
										</h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>C++ Ethereum is the third most popular of the Ethereum clients, focusing on code portability to a broad range of operating systems and hardware. The client is currently a full node with transaction processing based synchronization.</p>
										<br/>
										<p>To run a cpp-ethereum node, download <a href="/{{.CppGenesis}}"><code>{{.CppGenesis}}</code></a> and start the node with:
											<pre>eth --config {{.CppGenesis}} --datadir $HOME/.{{.Network}} --peerset "{{.CppBootnodes}}"</pre>
										</p>
										<br/>
										<p>You can find cpp-ethereum at <a href="https:
									</div>
								</div>
							</div>
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2>
											<svg height="14px" version="1.1" role="img" xmlns="http:
											Ethereum Harmony<small>Third party Java client from EtherCamp</small>
										</h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>Ethereum Harmony is a web user-interface based graphical Ethereum client built on top of the EthereumJ Java implementation of the Ethereum protocol. The client currently is a full node with state download based synchronization.</p>
										<br/>
										<p>To run an Ethereum Harmony node, download <a href="/{{.HarmonyGenesis}}"><code>{{.HarmonyGenesis}}</code></a> and start the node with:
											<pre>./gradlew runCustom -DgenesisFile={{.HarmonyGenesis}} -Dpeer.networkId={{.NetworkID}} -Ddatabase.dir=$HOME/.harmony/{{.Network}} {{.HarmonyBootnodes}} </pre>
										</p>
										<br/>
										<p>You can find Ethereum Harmony at <a href="https:
									</div>
								</div>
							</div>
						</div>
						<div class="clearfix"></div>
						<div class="row">
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2>
											<svg height="14px" xmlns:dc="http:
											Parity<small>Third party Rust client from Parity Technologies</small>
										</h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>Parity is a fast, light and secure Ethereum client, supporting both headless mode of operation as well as a web user interface for direct manual interaction. The client is currently a full node with transaction processing based synchronization and state pruning enabled.</p>
										<br/>
										<p>To run a Parity node, download <a href="/{{.ParityGenesis}}"><code>{{.ParityGenesis}}</code></a> and start the node with:
											<pre>parity --chain={{.ParityGenesis}}</pre>
										</p>
										<br/>
										<p>You can find Parity at <a href="https:
									</div>
								</div>
							</div>
							<div class="col-md-6">
								<div class="x_panel">
									<div class="x_title">
										<h2>
											<svg height="14px" version="1.1" role="img" xmlns="http:
											PyEthApp<small>Official Python client from the Ethereum Foundation</small>
										</h2>
										<div class="clearfix"></div>
									</div>
									<div class="x_content">
										<p>Pyethapp is the Ethereum Foundation's research client, aiming to provide an easily hackable and extendable codebase. The client is currently a full node with transaction processing based synchronization and state pruning enabled.</p>
										<br/>
										<p>To run a pyethapp node, download <a href="/{{.PythonGenesis}}"><code>{{.PythonGenesis}}</code></a> and start the node with:
											<pre>mkdir -p $HOME/.config/pyethapp/{{.Network}}</pre>
											<pre>pyethapp -c eth.genesis="$(cat {{.PythonGenesis}})" -c eth.network_id={{.NetworkID}} -c data_dir=$HOME/.config/pyethapp/{{.Network}} -c discovery.bootstrap_nodes="[{{.PythonBootnodes}}]" -c eth.block.HOMESTEAD_FORK_BLKNUM={{.Homestead}} -c eth.block.ANTI_DOS_FORK_BLKNUM={{.Tangerine}} -c eth.block.SPURIOUS_DRAGON_FORK_BLKNUM={{.Spurious}} -c eth.block.METROPOLIS_FORK_BLKNUM={{.Byzantium}} -c eth.block.DAO_FORK_BLKNUM=18446744073709551615 run --console</pre>
										</p>
										<br/>
										<p>You can find pyethapp at <a href="https:
									</div>
								</div>
							</div>
						</div>
					</div>{{end}}
					<div id="about" hidden>
						<div class="row vertical-center">
							<div style="margin: 0 auto;">
								<div class="x_panel">
									<div class="x_title">
										<h3>Puppeth &ndash; Your Ethereum private network manager</h3>
										<div class="clearfix"></div>
									</div>
									<div style="display: inline-block; vertical-align: bottom; width: 623px; margin-top: 16px;">
										<p>Puppeth is a tool to aid you in creating a new Ethereum network down to the genesis block, bootnodes, signers, ethstats server, crypto faucet, wallet browsers, block explorer, dashboard and more; without the hassle that it would normally entail to manually configure all these services one by one.</p>
										<p>Puppeth uses ssh to dial in to remote servers, and builds its network components out of docker containers using docker-compose. The user is guided through the process via a command line wizard that does the heavy lifting and topology configuration automatically behind the scenes.</p>
										<br/>
										<p>Puppeth is distributed as part of the <a href="https:
										<br/>
										<p><em>Copyright 2017. The go-ethereum Authors.</em></p>
									</div>
									<div style="display: inline-block; vertical-align: bottom; width: 217px;">
										<img src="puppeth.png" style="height: 256px; margin: 16px 16px 16px 16px"></img>
									</div>
								</div>
							</div>
						</div>
					</div>
					<div id="frame-wrapper" hidden style="position: absolute; height: 100%;">
						<iframe id="frame" style="position: absolute; width: 1920px; height: 100%; border: none;" onload="if ($(this).attr('src') != '') { resize(); $('#frame-wrapper').fadeIn(300); }"></iframe>
					</div>
				</div>
			</div>
		</div>

		<script src="https:
		<script src="https:
		<script src="https:
		<script>
			var load = function(hash) {
				window.location.hash = hash;

				
				$("#geth").fadeOut(300)
				$("#mist").fadeOut(300)
				$("#mobile").fadeOut(300)
				$("#other").fadeOut(300)
				$("#about").fadeOut(300)
				$("#frame-wrapper").fadeOut(300);

				
				var url = hash;
				switch (hash) {
					case "#stats":
						url = "
						break;
					case "#explorer":
						url = "
						break;
					case "#wallet":
						url = "
						break;
					case "#faucet":
						url = "
						break;
				}
				setTimeout(function() {
					if (url.substring(0, 1) == "#") {
						$('.body').css({overflowY: 'auto'});
						$("#frame").attr("src", "");
						$(url).fadeIn(300);
					} else {
						$('.body').css({overflowY: 'hidden'});
						$("#frame").attr("src", url);
					}
				}, 300);
			}
			var resize = function() {
				var sidebar = $($(".navbar")[0]).width();
				var limit   = document.body.clientWidth - sidebar;
				var scale   = limit / 1920;

				$("#frame-wrapper").width(limit);
				$("#frame-wrapper").height(document.body.clientHeight / scale);
				$("#frame-wrapper").css({
					transform: 'scale(' + (scale) + ')',
					transformOrigin: "0 0"
				});
			};
			$(window).resize(resize);

			if (window.location.hash == "") {
				var item = $(".side-menu").children()[0];
				$(item).children()[0].click();
				$(item).addClass("active");
			} else {
				load(window.location.hash);
				var menu = $(window.location.hash + "_menu");
				if (menu !== undefined) {
					$(menu).addClass("active");
				}
			}
		</script>
	</body>
</html>
`



var dashboardMascot = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x01s\x00\x00\x02\x00\b\x06\x00\x00\x00p\xe4\x8c`\x00\x00\x00\x06bKGD\x00\xff\x00\xff\x00\xff\xa0\xbd\xa7\x93\x00\x00\x00\tpHYs\x00\x00\v\x13\x00\x00\v\x13\x01\x00\x9a\x9c\x18\x00\x00\x00\atIME\a\xe1\x03\x1d\x0e0&\xf3\xca\t\x11\x00\x00\x00\x1diTXtComment\x00\x00\x00\x00\x00Created with GIMPd.e\a\x00\x00 \x00IDATx\xda\xec\xbdw|\x15U\xfe\xff\xff<3s[\x12BB\x12H!\x90\u0411&H\a\x01i\"V\xb0!b\xc1^>\xbb\xee\xae\xdd\xd5u\xed}-k/X\u058e\xa8\xa0\u049bH\x87\bH\xef\x04BI\x81\xf4[\xa6\x9c\xf3\xfbcnB`\xdd\xef\xcf\xdd\xd5\xddU\xe7\xf9x\\\u023d\u027dw\xee\u0339\xafy\xcd\xfb\xbc\xcf\xfb\r\x1e\x1e\xff\";wn\x17\xb7\xdf~\xabQw_)\xa5+\xa5\x1a)\xa5\u0494R\xc9J\xa9D\xa5T\xf0\xb8\xa7\xe9\x1f|\xf0\x9eQYyXx{\xd0\xc3\xe3\xc7\xc3\xfbBy\xfcK\xbc\xf3\xce[\xe2\xd2K/Wq\x11o>y\xf2G'\xaf[\xb7\xae\xff\u05ad\xdb2\xa5\x94\xa9\xd1h\xb4:---\u06b9s\xe7HJJ\xf2\xb2f\xcd27\x9e}\xf6\x98\"!\xc4\u07ba\xd7x\xf6\xd9g\xb4\xcb.\xbbD\xa6\xa4\xa4\xb1d\xc97\f\x18p\xb2\xb7c=<<1\xf7\xf8O\xf0\x97\xbf<EII\x89x\xf4\xd1\xc7\x14\xc0\x8c\xe9_\x0e\xffv\u035a\xa7\u05ec]\u05f9`\xf5jJK\xcb0M\x13M\xd3\b\x85B4i\x92J\xabV\xf9\xe4\xe5\u54d9\x99Y\x90\x9a\x9a2\xa5_\xbf\xfeS\xfb\xf5\ubfe9\xee5o\xba\xe9V\x9e}\xf6\t\x94R\b\xe1\rI\x0f\x0f\x0f\x8f\x9f\x9c\xf7\xde{\xb7\x81;\u007fs\xe4\xc4+.\xafi\u04e6\x8d\x02\xfe\xe1M\xd75\x95\x9c\x9c\xacZ\xb7n\xadF\x8d\x1a\xa5\u018d\xbbp\xd7=\xf7\xfc\xf1\xf5\xaf\x17.\x18q\xfc\xeb\u007f\xfa\xe9'\xdeN\xf6\xf0\xf0\xf0\xf8\xa9\xc9i\x9e\v\xc0\x97_N\x1bq\u0265\x97\x1c\xceh\u06b4^\xb4\x85\xa6)\xc3\xe7S>\xbf_\x89\xef\x15u]\x86B!\x99\x93\x93\xa3\xfa\xf6\xed\xab.\xb8\xe0\x82\x8a\a\x1e\xb8\xef\x83}\xfbv\xf7k\xf8\x1e\u007f\xf8\xc3\x1f\xc4\u0739\xb3\xbc\x9d\xed\xe1\xe1\xe1\xf1S\xb0r\xe5r\r@)\xd5\xee\xf6\xdbo\u06d2\u07fa\xb5\x02$\xa04]W\x86\xe1S\t\u024dUjV\xaeJ\xcdn\xa9\x12S\u04d5\x10\xe2\x18A\x17B(\xbf\xdfo'5jde\xe74W\xfd\xfa\xf5WW^yE\xe5k\xaf\xbf\xfa\xa0R*\xb1\xee\xbdz\xf7\xee#\xdex\xe35o\xa7{x\xfc@\fo\x17x\xfc\x10\x94R\x9cz\xeaH\x00\xfe\xf6\xb7\xb7\xaf\u0634yK\xfb\xdd;w\u0680\xa1\xe9:\x86\xcfO\x93\xdc\xd6t\x18r\x06\x19m\xbb\xa0\xe9\x06f\xd5\x11\xcavmd\xff\xa65\x1c\u06be\x91pU\x05J)L\xd3\xd4\x1d)\xb1\x1d\u01e9\xae\xae\xe1\u0421\x83\u027bw\xef\xfe\xe3\x96\xcd[\u03999s\xfam\xa3F\x8d\x9e\xber\xe5\n\xb5r\xe5\n>\xfa\xe8\x03q\xe1\x85\x17)\xef\bxxxx\xfc\b\u0318\xf1U\x9d+O\xbe\u66ab\xe7\xb6\xcc\xcbW\x80\x85\x10J7|\xaay\x97^\ua497\xa7\xab{\xd7Ku\xcfw\x8e\xbao\x83\xa3\x9e\xd8\"\xd53[\x1d\xf5\xe0\xd2\xfd\xea\xca\x17>Q\xfd/\xbcZ\xa5\xe5\xe6\xff]X&!1Q\xa6\xa5\xa5\xa9\xf6\xed\u06ebs\xce9[=\xfa\xe8#\x1f\u06b6\x99Y\xf7\u0793&\xbd\xe1\x99\x0e\x0f\x0f\u03d9{\xfc\x18l\u0630A\x03\xe4\xf4\xe9\xd3z\x95\x1d>\xd2\xfdPq1\x80.4\x8d`R2\x9dF\x9eG\xab\xc1\xa7\x11\x8dJ|\xd2B\xf3\t4\f4M#%3\x9b\x93\xce<\x97\x13O=\x97\xc2u+Y\xf1\xc9$VM{\x8fhm\r\xb6\x94H\xc7\x11JJu\xe8\xd0!QYYIY\xd9\xe1\v\v\v\v{\xbf\xfa\xea\xcb\u007f\xbc\xfa\xeak?\x12B\xd8c\u019c\xa3=\xf0\xc0\xfd\xaas\u7b9eK\xf7\xf0\xf0\xc4\xdc\xe3\xdf\b\xb3\u803dq\u00e6v\xa6i6\x89E\xc2\x12\xd0p\x1c2\xf2\xdb\u04a6\xcf`t\xc0pb\x04|\x02\x03\xb0-\vi\x83\xa6\t\x10\x02C\xf7\u0476ooZu\xefM\x87\x81#\x98?\xe9iv\x16,AJI$\x12\x11J)4Mc\u02d6-\x1c<x0\xff\u0421C\xefWTT\x9c\xa7\x94\xfa\x83\x10\xa2\xf0\xb3\xcf>\xe7\xc9'\x9f\x10II\x89\xea\xba\xebn\xf0\x0e\x8a\x87G\x03to\x17x\xfc\x10\xe6\u0319\xeb(\xa5\xf4\x85\v\x17\\\xb5x\xf1\u0493JJJ\x00\x84\xa6\xe9\xe4\x9f4\x90^\xe7\\\x8a\xdf\x1f\xc4'\x1c4\x01 @\x80B\xe1(\x85t\x14\xb6\xed`\xc5\x1c\x84\xd0\xc9\xed|\x02\xed\xfb\x8d\xc4\x17\brp\xebz,3\x86m\xdbH)\xd14\r\u02f2\u063f\u007f?\xd5\xd5\xd5\x1d\xb7n\xdd:\xfc\xfd\xf7\u07dd\xf3\xf4\xd3\xcf\x1c\x99={\x0eYYYb\xfc\xf8q\u031e=\xc7;0\x1e\x1eq4o\x17x\xfc\x13\xe4X\x96\u0569\xa2\xa2<\xae\xd6`\x04\x83\xa4d\u5498\x92\x8a\x81D\x17u+\xd1\x14(\x85T \x158\xb87KB,\x16\xa5\xb6\"L\x93\x9c\xe6\x9cu\xcb\xc3\\\xf9\u0707\xb4\xee\xd1\x1f\x00\xd34\xa9\xad\xad\xc54M\xa4t\xd4\u06b5\xeb\x983gn\xe7G\x1ey\xec\u04d2\x92\x03\xb9\x00\xaf\xbd\xf6\xba*--\xf5V\x17yxxb\xee\xf1\xafP[Se\x16\x15\xed\x8fX\x8e\x13Wk\b$$\xd18\xb39\x9a\x06\x02\x85&@\x8b\vz\xc3\u015c\ueb27B\x01\xb6\u0490B'\\S\x8be\xd9t\x191\x8a\u02df\xf9\x80\xbec/\x03\xc0\xb6mjjj\x88FcB\b\xa1v\xed\xda\u0152%K\xbb\xfc\xe1\x0f\xb7~\xban\u0777-\x00\x1e{\xec\t5c\u0197\x9e\xa0{xxb\xee\xf1\xcfR\xb4\xbfH\x84\xc3a\u0371\xed\xfa\xc7|\x81 \xa1\u4538\x11W8J \x95\xa83\ueba0\xbb&\x1d\xea\\\xbar\x1d\xba\x12:\u04b1\xa9\xad\xb6i\u05a6\x05\xe3\x1fy\x83\xb3n~\x88P\xa3d\xa4\x94\xd4\u0506\tG\"BJ\xa9\n\v\vY\xb7n]\xcfG\x1ey\xec\x8b\x193\xbe\xec\fp\xdaig\xa8\r\x1b\xbe\xf3B\x85\x1e\x1e\x9e\x98{\xfc3\xa4\xa4\xa4\xa0i\x9aRR\x1e}P\b\x94Tq\xe7-\u0730J\u0703\u02fa$\u0138So\x98\x86\"\x15\xd8R\xa0\x14(\xe9P]a\xe2O\xd09\xf5\x86\xbb\xb8\xe0\xde\x17H\xcdj\x8e\x92\x0e\xe1p\x98H4*\x14\xa8\x83\a\x0f\xb1a\u00c6\xae\xef\xbf\xff\xc1\x97\v\x16\xcc;\x1f\xa0s\xe7\xae\xce\u0319\u04fdq\xec\u1279\xb7\v<~(\u035ae\xeb)\xa9\xa9\xbe:\xd3\r`\x9b&\xe1\x8a#q]\x17\xf5\x95\xdb\xd4q\xff\x1f\r\xb7\xa8\xfa\u01dc\xb8\x93\a\xd7\xd5\xd7VE\xb1\xa5\xa2\xf7y\x13\x18\xf7\xd0\xeb4ks\x02(E4\x12!\x12\x8d\n\u02d1\xeaPq\t\u02d7\xafh\xf9\xe1\a\x1f~<s\xe6\xf4\xdb\x00F\x8d\x1a-\xdfy\xe7m\xf1\xd9gS\xbc\x83\xe4\u1279\x87\xc7\x0f\xa0*))\xb1\xd8\xe7\xf3\xd7\xeb\xb4\x15\x8dPY\xb2\x1f%\xeb*\x1e\xaa\xb88\xbb\xee[\xd5\u074e\x13\xf6\xba\xfb\x96t\x1d\xbc\x16\u007f\x8e\x15\x8b\x11\x8b\xdat\x1av*\xe3\x1ez\x9d\xdc\u03bd\x000\xa3Qj\xc3a\x11\xb5lJ\x0f\x973c\xe6,\xde\u007f\xff\x83\u01fe\xf8b\xeam\x00\x97^z\x99Z\xbf~\x83\x98<\xf9#\xef(yxb\xee\xe1\xf1\x8f0\f\x9f.\x84\xa8\xb2L\xab --\r\xdc:+8\xb6EU\xc9Aj+\xcaA\xd3P\xc7LI\n\xb7p\xcbqB~\u053d\v$\x02[\xb9~]\x13\x02M\b\xa4m\x13\xad1i\u0777\x1f\x17>\xfc:\xad{\x0f\x06\xc0\x8eE\x89\x86\u00d8\xb6Cyu\r\xf3\xe7/\xe0\x93O\xa6<\xb6h\xd1\u009b\x00\xfe\xf4\xa7{\u055a5k\u0164I\xaf{\a\xcc\xc3\x13s\x0f\x8f\xef\xe3\xfe\a\xfe\xac\x03\xb4i\u04fa\xbcY\xb3f\xf5\x9a,\x1d\x87\xaa\u0483T\x1c\xda\x17\x9f\xect\xeb%\xba\x99,\xea8\xf9v\u007f\xaf\xe2.\xbc\xee/l)p\x94h\x10W\x17\xeeB\xa2\xea(\xb9\u077ar\xc1C\xaf\xd3a\xd0i\xae\xa0\x9b1\xccH\x18\u01d1T\xd4\u052a\xb9\xf3\x17\xf0\u059bo=\xb3`\xfe\u071b\x00\x1e~\xf8\x11\x15\x8b\u017cq\xed\u1279\x87\xc7?\xc0\x01h\u07fe\xfd\x16M\xd3\x0e\x01B)\xa5P\x8a\xea\xb2b\x8e\xec\u0749\xe1sE\xbc\xae\xc1\x84\x10 \x8e\x9f\xfa\x14\xaa~R\xb4>$\x03\xd8Ra\xab\x86a\x18\xe1\xc6\u02ebbd\xb5o\xc3y\xf7\xbfD\x87\x93G\xb9\x82n\x99\u0122a\x1c\xa9DyU\x8d\x9c5g\x1e\x93&\xbd\xf9\xccg\x9f~\xf2;\x80\ubbffQ~\xfa\xe9'\xde\xd8\xf6\xf0\xc4\xdc\xc3\xe3x\xfa\xf4\xee\xad\x00\x86\x0e\x1d\xbe==#cKBR\x12\xae\xc1\x96\xd4\x1c.\xe6\xd0\xf6\x8d\xf1\xa5\xfb\xc7\x0f\xa9\xe3\x03,\xc2\x15\xf4\x06\x8f\v\x14J\x81\x1d\x8f\xb3\x83B\x13\n\x81\xe6\nz\xb5Ef\xbb\x96\x9c\xff\xd0kt\x19y\xae{f1M\xccH\x18\xe98Zyu\xad\x9c\xbb\xe0k>\xfc\u88e7\xdf{\xf7\x9d\xdb\x01\u018e=O\xae^\xbd\xd2K[\xf4\xf0\xc4\xdc\u00e3!C\x87\x0e\x97\x80\x10B\x1c\xc8i\xde|k\x8b\xdc\\\x00[I\x89\x15\rS\xbag;U%\xc5\xe8\x86\xcfm\xff\u01b1\xee\\\x1c\u04e1P\x1c\xa3\xf3*\x1e\x9d\xc15\xfa\rD\x1f4\xcdM]\xac=b\x91\u05a29c\xef}\x81\ue9cfs\x05\xdd2\xb1\xcc(J)\xad\xaa6\xa2\xbeY\xb2\x8c\xaf\xa6O\u007ft\xea\xd4Oo\a\xe8\u0673\xb73c\xc6W\xde\xc2\"\x0fO\xcc=<\xea\xe5W\b\x9ey\xe6\x19\r\xa0u~\xfe\xdaF\x8d\x1b\x03\xf8\xa5m#\x1d\x87\xe2]\x9b9\xb4s\x13\xbe@\xbd<\xd7GR\xc41.\xbdAz\v\r&G\xd5\u047fQ\xf5r\xee\xfe\xa4\t\x01\xca!V\x15%-\xa7\x19c\xee~\x8e\xeeg\\T\xef\u042dh\x18)\xa5\xa8\xac\x8d\xb0h\xf1R\xbe\xf8\xe2\xabG\xe7\u039d5\x01\xe0\xb4\xd3NW\x0f<p\x9fX\xbdz\xa5w\x10=<1\xf7\xf0\x00\xc8\xcfo)\x01.\xba\xf0\xbc\x95\xe9\xe9\x19\xbb}\xfe\x80+\xbfJq\xa4h\x0fE\x1b\v\xb0M\xd0u\xbd\x81\xcbv\xe5\xf9\xe8\x9a\xd0:\xbb\x1e\x97\xeacD\xfd\xa8\x84\xcb\xe3\x025Z<\x16\x1f\xae\x8a\x92\x9c\x95\xc1\xd9w=\u0349\xa7]\x00\xb8\x93\xa2V4\x8cr$\xe5\u0575\xcc_\xf05\x1f~\xf8\xf1\x13\u02d6~\xd3\x15\xe0\x9e{\xeeU_\u007f\xbdH\x1c<\xb8\xdf;\x88\x1e\x9e\x98{x\x9cr\xca)\n \xb5I\u01b7\x1d\u06b7\u06d9\x95\x95\x05\xa0\xa4ccFj)\\\xb3\x8c#\xfbv\x11\b\xf9Q\xaa.\xa5\xa5\ue98e\xae5\xaa_\x19ZW\x95+.\xe0\xf1\xa5\xfeu\x81\x96\xfa\xfctE\xbcD\x80[N7Z\x15%5\xbb\x19g\xde\xf54\x9d\x86\x9fsT\xd0\xcd(JA\u0251\n\x96._\x99\xf9\xde\xfb\x1f\xbe\xab\x94\xca\x03\xb8\xf9\xe6[\u051bo\xbe\xe5\x1dD\x0fO\xcc=<\x92\x93S\xea\u007f\xee|B\x87\u03dbff\x02\b\xe56\x98`\xef\x86\x02\xf6\xac]\x1e\x17]\xcd\xf5\xe4B\x1d\xe3\xb0\x05G\x8bn\x1d\x1bEw\x05\xdd]\x15\x1a\x1f\x9c\xc2\x1d\xa0\r\x9f\x03\nM\x83hu\x94\xf4\xdcl\u03be\xebiZ\xf5r\xf3\u042dh\x04;\x16E*\xa5\xf6\xec\xdb\xcf\xeao\xd7ty\xe4\xe1\x87\xea\x15\u0732L/~\xee\xf1\x8b\u015b\xed\xf7\xf8\x97\x98:u\u06bey\xf3\xe7\x8f(\xda\u007f 3\x16\x8d\x80t\xb0bQ\x1c\u01e1y\xe7\xde$\xa7\xa5#m\xbbA\xce9GeY\x88\xfa\xf4E\x94:\xb6\xbcb\xbd+\x17\xf5\xa1\x97\xba\x12\x00uN]w\xb3\x16\xb1mh\u04bc\t\xcd\u069e\xc4\xfe\xf5\xab\xa9*9\x80\xe3\xd8h\x9a&\x10B\x95\x1d>,L3\x9a\xf7\u007f7\u07980{\xf6\xec\xb9\v\x17~\r\xc0k\xaf\xbd\xca\x17_|\xe1\x1dD\x0f\u03d9{\xfcz\xb9\xe1\x86\xeb]\xc1\x15\xe2\u0420\x81\x03\x16d\xba\xee\xdc\xcd\x15\xb7,\xf6|\xbb\x94\xdd\x05\x8bQJa\xf8|GE\xbb>\x98\u00b1\xf1\xf3\xfag\x1f]`TW\xac\xcbQ\x02[\x1d\x9f\xc4\xe8.0B\b\x94c\x13\xaeth\u0465\vg\xdc\xf9\x14i-\u06c0RX\xb1\bJ\xda\xc2r\xa4Z\xb7~\x13\x05\x05\x05\xb7\u035e=}\f\xc0\x94)\x93\xf5\xab\xaf\xbe\xc6;\x90\x1e\x9e\x98{\xfc\xba\xb9\xf6\u06ab\xeb\u007f>}\xf4i\x9ft<\xa1\x83\n%$\xba\x8a\xac$\xe1\x8a26\xcd\xff\x9c\u0292\x03\xf8\x03:\xba\x00\xedh\xbb\x8a\x06\x01\x95\x06%\x15\x1b8\xf2\xba`\x8a#AJ\x15\xaf\xaa\x18\xbf\xc5\v\xbe\xd4M\x90\n\x01\xca6\x89\xd5Z\xb4\xedw2\xa7\xfd\xfea\x12S\u04d0\x8e\x83m\xc6\x10(Q\x13\x8eX\x9b6oe\xc6\xf4YW\x01\x9c{\xee\xf9\xce\u0295\u02fdq\xef\u1279\u01ef\x9bn\xddz0y\xf2\xc7\x00\xe4\xe7\xb7^\u04a3[\xb79\xd99\xd9\xf5Z,m\x8b\u00b5+(\\\xbb\x1c!\xc00\f4\xa1\x8e6\xach \xe0\xa2\xc1?nhE\"m\a\u01f4pL\x13\u01f6\xdd>\xa2\xf1a\xea\x98Ql3\x86m\xd9HG\xba\xb1u!P\x8e\x85\x15u8q\xf4\xf9\f\x9ax\v\bp,\v\xdb4\xd1\f\xdd\xd8Y\xb8\x97\xad\xdbw\x8c\xfe\xf8\xe3\x0fo\x04\xe8\u077b\xaf\xfa\xee\xbb5\xde\xc1\xf4\xf0\xc4\xdc\xe3\xd7\xcd\xf9\xe7_P\xffs\xcb\x16-\x1e\xec\u043e\x1dqw\x0e@e\xf1~\xd6N\xff\x98\u02b2rt\xbf\x8e@\xb8\x0e\xbd~y\u007f\u0709\xd7?\xa6PR\"-\xcb\x15k3\x86c\x9b\x14\x16,a\xf6Sw1\xf7\xaf\xf7rp\xdb\x06\x84\xeesE:\x1a\x8d\xf7\fu\x90\n\x84\xa6\xe3X&\n\xe87\xfez\xba\x8d\x1eW/\xfe\x8ei\nG*\xb9u\xfbN\xbe-\xf8\xf67J\xa9\x96\x80\x9a9s\xb67\x19\xea\xf1\x8b\u009b\x00\xf5\xf8\x97\x989s\x86x\xf7\xdd\xf7\xf8\xec\xb3\xcf\x0f.[\xb6\xb4WQQQ\x9b\xca\xca\xca\xfa\xdf\xd7\x1c)\xa1i\xab\x0e4?\xa1\x13\x8eS\x97\x9a(\xe2\xf9\u3abe\xad\x9c\x06(i#-\vi\xdbH\xdbB)\x89c[\xac\xf9\xfcmV|\xf0\x02\xfb\xd6-\xa7t\xf762\xdbu%9#\x13;\x16E9\x0e2\xde$C\x17\x02M\u05f0-\x8bPr\x12\xa9-:\xb0k\xd5\"j\x8f\x94 \x1d\x1b\xc3\xf0\x8bp4\x8a\xa6\x91n\x9b\x91\xa2i\u04fe\\6g\xce\\f\u039c\u03bb\xef\xbe\xe7\x1dL\x0f\u03d9{\xfcz\x195\xea4w\xe1\xbd\x10V\x97\u039d\xa6\xb6i\u075a@0T\xff\xfb\xda#\xa5\xac\xfd\xea#*\x8b\xcb\xf0\x05\xf4z'\xae\x8b\xa3\v\x88\x04\xa0\xa4\x04\xdbF\xd9\x16\u04b6\x90\x8e\x83\xb4,t\xdd 5;\x8fPr*\x00\xbbW.\xe4\x9bIOR]V\x8c\xee\xf3!-\x13\u01ccaG\xa3\u0626+\xee\x9a\xd00\xc3&M;t\xa6\xcfe7\xa3\xf9\xfc()1c\x11\xa4\x92l\u0676\x83\xd5\x05k\xae\t\xd7V\xe7\x02L\x9c8\xd1s\xe7\x1e\x9e3\xf7\xf0\xa8\xe3\x8b/\xbe(.(X5\xbch\xff\xfef\xb5\xe1\bR:\x00T\x1c\xdaKJ\xb3\x1cZ\x9e\xd8\vi+\x94\x92\xf5\x8b\x81\xa4\x02\xa9\x14\u02b6Q\xb6\x8dt,\x1c\xc7A92\xbezT\x91\x9e\xd7\x16\xcd0(\u0779\x19\xc74)?\xb0\a#\x98@\xf3N'!m\x1b%%B\xc9x-\x18\x88w\x95F\t\x1f)\xf9\x9d)\u07fb\x9d\xd2\xed\xebQn\x03j,[\xe2\xd8vZ\xf9\x91\xb2\xb2\xf9\xf3\x17|SSS\x8bR\x92\xfb\xee\xbb\xcf;\x88\x1e\x9e3\xf7\xf8\xf5\xb2c\xc7\u05b89\xd7\xf6\xf5\xec\xd9\xf3\xf5.\x9d:\x11\b\x06\xd0\r\x03\x003\\\u02ea\xcf\xdf\xe1\u040e\x1d\x18A\xb7\x00\x97T\x10\xb3%Q\xcb\xc1q$B)7\\\"\x15\x9a\xaa\x13g\x89c\x99\x18\xfe }\xc6]\xcf\xc0\x897\u04e8i6V4\xc2\xe6yS)Z\xbf\x1a\xdd\x1f@96R\xd6M\x9a\x9a(+\x86\x94\x92X$\x8a\x11\xf4\xd1k\xfcoIHM\a\xeaV\x88\xc6\xd8[T\xc4\xe6-[\xafUJ\xba+\x9e\x84\xf7\x15\xf0\xf0\x9c\xb9\u01ef\x9c\xe7\x9e{\x9e/\xbf\x9cf\xbc\xff\xfe\a\xf2\xb3\xcf>\xdf\xf5\xdd\xfa\xef\x86\x1e<x(\xbb\xaa\xaa\xcaQRjJ)*\x0f\x15\xe1\x0f%\u043a\xcf0\x14\x02\u06f6\xb1\xe2\xe9\x85~\rt%\x91\xb6\x8d\xe3\xd8 \x9dz\xc1\x17\n\xa4c\xa3\x19\x06\xd9't\xc7\x17J\xa0d\xc7F\xaa\x0e\x15\xa1\x80\xecN=1\x82!pl4\r\x10\xae\x9b\a\x81\xe9\x80\x14\x06\x8d\x9b\xe6Pyp/\a7\xaer\xdd~<\x97\xc6\xd0\xf5\u0186&J\xa7N\x9d\xb6\x04`\xf1\xe2EL\x9a\xf4\xa6w@=<g\xee\xf1\xeb\xe5\xf4\xd3\u03f4;th\xaf\v!JO<\xb1\xdbcy-[\u0621PH7|>Y\xe7zWN\x99\u0136of\x12Lr\x1d\xbbO\x17\x04}\x1aZ\xbc\u035c\xc2\x15b'\xde4T(U\xbfX\u050aF\x90\xb6M\x97Q\xe7\xd3\xed\x8c\xf1\x18\xc1\x10;\x97\xcfg\xc7\u04b9h\x9a\x8e\x00\xa4\x94\u060eD:\xd2\x15w\xe5\x80\x19!\x10\xd4\xe8|\xda8B)\xae;\x97\xb6I,\x16\xa5\xb8\xb8\x84\u056b\v\xaePJ5\x05\x188p\x90w =<1\xf7\xf8al\u07bc\xe1\u07dal[\xb3\xa6\xe0\u007f\xb2\xea\x9f\x10\x82\u0673g\xba\xd5\x14/\xbaxr\xabV\xf9S\xf3Z\xb6@\xd34G\xb8\xa5\x0e\xa99\\\u02827\x9e\xe4\xf0\xde\x03$$'\xa0\xa1\u0705D\x02\x948Zd\xcb\xed\x17\x1a_\xbc_W\x17W)\xcch\x18!4:\x8f<\x8f\xfc\x9e\x83\x88V\x1da\xc3\xec)\x1c.\u070e\x11\f\xe1H\x85\x94\xf1\xd5E\xd2A96\xb6\xe3\x10\x8bJ2\xdau\xa5\xc3\u0433\u074dU\n\u01f69|\xa4\x9cM[\xb6\xb4\xff\xec\xd3O\xeas,\xa7O\xff\xd2\x1b\xa4\x1e\x9e\x98{\x1c/\xdc\x1b\xffN\xb8;v\xec\xac\xfe\x9d\xd7\xec\xde\xfd$\x95\x95\x95\xf3w\x8f\xef\u06f7\xe7\xbf\xfaY\x8b\x8a\xf6\u04a2E\xbe\x9a>\xfdK\r\u0b33\u039a\x94\x97\x97\x17m\x94\x94\xe4\xf3\x19>U\x97W\xbec\xf9|\xbe\x9e\xf4$2f\x13LH<\xdaRN\xd3\x11\xba\x01B\xe0H\x85-\xdd0\x8b\x8aOl\xd6aE#4JoF\xd7\xd1\xe3Hm\u078aC\u06fec\u04c2/\x88\x86k1|~\x90\x12\u02d1\x98\x96\x83m\xdb\b\xe9\xe0\xc4\xc2\x04\x1b5\xa6\u07503\xf1\x05\x13\\w\xee\xd8D\xa2QJJ\xcbX\xb0p\u163a\xd7\x1f=\xfa\fo\xe0z\xfc\xac\xf1R\xb3\xfe\r\x94R\u031f?W[\xbbv\x9d\xb6k\xd7N\xfd\x85\x17^\x8a\x1d\xf7\xfb\x04)\xed\xc0\u0739s\xd2g\u03de\xa3L\xd3N\xea\u07ff\uf165\xa5\xa5I\u06f7\uf215\x96\x96\x12\x0eG\xb0m7\x93C\xd7u\f\xc3 11\x91\xa6M\x9b\x92\x97\x97\xe7\xcf\xc8H\x0fo\u077au\xea\x91#\x87\xcbN9\xe5\x14\u0577o\u07f2\x9c\x9c\x16\xa6\x10\"|\xdc\xe6\x18\x8f=\xf6\xa8\xc8\xc9\xc9v.\xbe\xf8\x12)\xc4\u007f\xef\xd0>\xf8\xe0\xfd\x0f|\xf5\xd5\xf4\xbb7n\xdeJ$\x1cV\x96e\n\xe2\x19'g\xdd\xf9\x17\x06^z\x13\b\x81m\u0190\nl\xd3$V[K4\x1aA\xd96B\x88x\xfdrw\x88\xaa\xb8\xab\xd6t\x1dG\x18,\xfb\xf8u\xd6M~\x89\x84\x944\x86\xff\xdf}\xb4\xee=\x18+\x16\xc1Q\x02\x89@7\f\x94\xe1G\xe9>\xfc\t\x89\u012a\x8e0\xe3\xc1\xeb\xd9<\xf73\x00t\u007f@\xa56N\x16}\xfb\xf4\xae\xbe\xf7O\xf7\\\u052bw\u07ef\x00\xbe\xfbn\r]\xbbv\xf7\x06\xb6\x87'\xe6\xbf&\xbe\xfa\xea\v\xed\x0f\u007f\xb8\x85\xad[\xb7\xca\x06\xe2ml\u0630\xb6\xcd\xe4\u025f\xa4WUU\xb7\xf6\xfb\x03\xc3\xf7\xed+\xca\b\x04|\x03\x0e\x1e<\x94\xb4\u007f\xff\x01\xc2\xe10\xb1X\x94X\xcc\xc4i\xb0\xf0E)U_\x94J\xd34\fC\xc7\xe7\xf3\xe1\xf7\a\b\x85\x82dee\u046cY\xb3\x9ah4\xba8''\xa7\xaci\u04cc-J\xa9UC\x86\f\xd9;h\u0410\"!DM\xddv\xa4\xa5\xa5j/\xbe\xf8\"\x17\\0\xee\xbf\"\xeaJ\xa9\x8c\x1b\u007f\xfb\xdbY\u04e7\xcf\xe8^\\\\\x8c\x15\x8bb[\x16\x00\xa1F)\x8c\xfd\xd3_\xe9u\xee\x04\xa2\x11\a\xe9\xd8\u0636\x85Y[K,\x12\xc1\xb6,\x84R\b];Z\x90K\xc5\xc3/Rb\x1a\t\x1c\u06b7\x97o'=\xc8\xfe\xb5\x8bi?\xf8L\x86]\u007f7\xc1\xe4\x14b\xd1\x18a\a\xfc>\x03\u007f\xc0\x87\xa3\xfb\xd14\x8dPJ\n\x05\x1f\xbd\xca\xf4\a\xae\a%\xd1t\x03\u007f  {\x9d\xd4C\x1b}\u06a8G\xee\xbc\xeb\x8fwy#\xda\xc3\x13\xf3_\t\xf3\xe6\xcdf\u0630\x91L\x9a\xf4\x12\x13'^W/\xbcJ\xa9\xd0\xec\xd93\xfb\u035a5+\xaf\xa4\xa4\xb4\xb7eY\xa7TTT\xb4\u06f3\xa7\x90\x92\x92\x12,\xcbB)E0\x18\xa4q\xe3\xc6$''\x93\x9c\u0708\x84\x84D\x12\x12B\x04\x02\x01t]G\xd7uw\"\u03f6\x89FcD\"a\xc2\xe1\b\xd5\xd5\xd5TUUQQQA4\x1a\x05\xc0\xe73h\u04a4\t\xf9\xf9y4i\x92V\u0628Q\xa3e\u035b7_\u06f6m\x9b\xe5\x13&\\\xbaR\b\x119\xea\x92\x1f\xc4q\x14\xfd\xfa\xf5d\xe4\xc8\xd3~\xd2}t\xf7\xddw\xf1\xe0\x83\x0f\x030k\xd1\xe2S\x9e{\xfa\x99\x99\u02d7.\xf6\xd7VWc\u01a2\xc8x\xbewjV.\x17<2\x89\x0e\x83\x87\x13\xad\xb5\xb1\xad\x18v$\x82\x15\tc\x99\x16\x8eTG\xfbV4\xa8\xb4(\x95\xa2*\xea@0\x89\x03K\xbed\u065b\x8f#\x1d\x87\xc1W\xdfA\xe7\x91c\x89DbT\xc7,\x82>\x83P\u0407\xd2\xfdh\x86N\xa0q\x13\x0emY\xcb\u053b.\xa3x\xdbw\bM\xc3\x1f\bZ\xad\xf3\xf3|C\x86\f\xfa\xfa\x85\x17^\x1a%\x84\x88\x1e<\xb8Ode\xe5*o\xb4{xb\xfe\v\xa4\xa4\xe4 M\x9bf\xfd\xdd\xe3{\xf7\xee\xea2y\xf2\xa7\x97-_\xbe\xbc]MM\xcd\xe0\x8a\x8a\x8a\xe4\xad[\xb7Q[[\x8b\xae\x1bdee\u04be};\xf2\xf2\xf2\xc8\xc9\xc9&++\x8b\x8c\x8c\f22\xd2INN&!!\x81P(\x84\xdf\xef\xbaGW\xcc\x15\x8ec\x13\x8b\u0148D\"D\xa3Q\xaa\xaa\xaa)++\xa3\xa4\xa4\x84\xe2\xe2b\x8a\x8a\xf6SX\xb8\x97\xed\u06f7\xb3\u007f\xff~b\xb1\x18\xc1`\x90\xb6m\u06d2\x9c\x9c\\\x15\f\x06\xbe\xee\xd9\xf3\xa4\xcdc\u01ce\xfd\xb8{\xf7\x93\n\x8e\xdf\xee\u077b\xb7\x93\x9f\xdf\xf6'\xd9W\x05\x05\xab\x985\u007f\xa1\xb8\xeb\xd6[\x15\xc0\v\x93\xdez\xf1\x93\x0f?\xbc~\xf5\u0295\u0122\x11e\u0162\xa2.\x0e\u07b2[\x1f\xce}\xe0Ur\xbbv%R\x15\u00caF\xb0\"\x11wE\xa7\xe3\xb8\xcd+\x94jP[\xd1Mo\x89\xda\x12\xcd\xf0#\xec\x18\x8b_\u007f\x94Ms>#\xf7\u013e\x8c\xb8\xe9A\x82\xe99T\xd7F0\f\x9d\u0120\x81\xe1\xf3#u\x1f\xba?\x00\x02f>\xfc\x1b\xd6|\xf6\xa6+\xe6\xfe\x80\u04f8qc}\xc8\xe0\x93\xf7>\xf5\xd4\xe3\x17\xe5\xe6\xe6/\xfd\xfc\xf3O\xc59\xe7\x8c\xf5\xc4\xdc\xe3g\x89\xe1\xed\x82\xefg\u0294\u027c\xf3\xce;\xa2i\xd3,u\xdc\xe3\xe7}\xfe\xf9\u0511\x97]v\xc5\u041a\x9a\xda\xd6;w\ue92a\xaa\x8aF\x8d\x1a\xa9.]:\x8b\x9e=O\xa2{\xf7\x1e\xe4\xe7\xbb\"\x9e\x91\x91Abb\xf2\xf1\x81\b@\x1d3\xc9W\xf7\xbf\x887np\xd3\xfa\xfe\xbe\xf2w8\xec\x8a\xfb\xc1\x83\a),\xdc\u02da5k((\xf8\x965k\u05a8\x8a\x8a\xaa\u4924\x843\xf7\xef\xdf\u007f\xe6\u0085\x8b.\xbc\xfc\xf2\xcbV\x8c\x181b\xf2\xc5\x17O\xf8\xa4\xee\xf9uB>s\xe6tF\x8d\x1a\xfd\xa3\uecd3N\xeau\xd4J\x037L\xbc\xec\xee\u28bd9\xd5\x15G\xce\u06bcy\xb3@\xf91c&\xa0(\\\xb7\x82\x99O\xdf\xcd\xd8\xfb^\xa2I\xf3\x1cl\xdbF7,\x94\xae\xa1\x94\x83-\x05R\xe0\x86\\\xe2\xcbF\x95\x02\xbf\xae\xa1\t\a\u007fr\n\x1dO9\x93\x03\x1b\v8\xb4e-[\xbe\x9eA\x971Wa\xe8\x1aR),[\xa1\xeb\x0eB\xd3Q\x8eE(5\x85\x8cV\x1d\xdd}-%RJ-\x12\x8b\x12\xae\xad\xcd]\xf4\xf57\ud065;v\xec\x10\r\xb7\xdf\xc3\xc3\x13\xf3\x9f1_}\xf5\x05\x8b\x16-\x12\xe7\x9e{\xfe\xd1\xc6\xf1J\xa5\xbd\xf9\xe6\xeb\x17O\x9d\xfa\xc5\x19\x0f>\xf8\xf0\xc9ee\xa5\xc1\xe2\xe2\x12\xfc~\xbf\u0763G\x0fN=u\x84\u07bbw/\x91\x9f\u07caf\u035a\x92\x94\u0538^\xb4m\xdb$\x1a\xad=&3\u37c9c\x1f\x15{\xb7\xff\xa5\xdf\xef\xa7E\x8b\x96\xb4h\x91O\x9f>\xfd9\xf3\xcc3(--e\xf7\xee=b\xe5\xcaUj\xee\u0739\xce\xea\u056b\u067auk\xcb\xed\u06f7\xb7\\\xbd\xba`\xf4\x84\t\x13\xee9\xf5\xd4\x113&L\xb8\xf4q!\xc4\x11\x80Q\xa3Fs\ubb77\x88!C\x06\xab\xd3O?\xf3G\u0747u\xf1\u007f!\xc4\x11\xa5\u0504CE\xfb>\x8aE\"\xa7\xed\u06bd\xcbM54-@\xb1i\xc1\x17$7\xcba\xf4\x1dO\xe3OL$b\x9a\b\xc3@H\x89\xa6\x1c\xa4\x12G\xd3\x14\x1b4\xa9PRa\x9bQ\xb2;\x9dD\xab\u07a7\xb0\xf6\u02ff\xb1c\xd9\x1c\x9au?\x99\xf4\xbc\xf6\u0122\x11L\xdb\x01M#\xa89 u\x90\x90\x9c\x99K09\x95hU9JI\x11\x8b\xc6,\xdbq|\xc5%\xa5\x1d\x00\xaa\xaa\xaa\xbd+U\x0fO\xcc\u007f\tTU\x95\x8b\xe4\xe4\u0506mox\xf6\xd9g\xae\xba\xf2\xca+\xee[\xb6ly\xf6\u07bd\x85\x84\xc3\x11\xb2\xb3\xb3\xcdq\xe3.\xd0/\xb8\xe0|\xa3{\xf7\x1edd\xa4\xe3\xf3\x05\x01\xb0\xed\x18\xe1p5J)4M\x8b\v\xb7@\xd3\xfe5\x9d8^\xf8m\xdb\u01b2,\xa4\x94\b!\xf0\xf9|\xb4h\x91O\x8b\x16\xf9\f\x18\xd0_\\~\xf9\xa5\xc6\xfa\xf5\x1b\x98:u\xaa3g\xce\\\xb9}\xfb\x8e\xa4\xfd\xfb\xf7w]\xb3fM\xd7\xf9\xf3\x17\\\xfe\xfa\ubbfd|\xe5\x95W\xdd/\x84\x90O<\xf1\xa4z\xe2\x89'9r\xa4T4i\x92\xf1\xa39R!\x04J)\xee{\xf0\x01\x9f\x10\xa2z\u03de\x1d\xf7\xff\xe5/\xcf\f\xa8\xae\xaeJ.--SJ)a\xdb6J)\x96\u007f\xf82\xfe\xe44\x06\xdf\xf0gT\xa0\x11\xb1\xa8\x89\xc2F)\a\x15oN\xd1\xf0\xe2\xa4.\xd5Q:\x0e\xbeP\x02\xed\x06\x8f\xa6p\xcd\x12\xcavof\xd7\xf2\xb9\xa4\u4dad\x0f\xc98\x8e\xc4r\x14>]\"\x1dE\xa3\xf4L\x12\x1a7\x89\x8b\xb9['\xecHy9\u06f7os\xdc\x10\xd4n\xefK\xe0\xe1\x89\xf9\u03dd\x993\xbf\u0493\x93S\x9d\xb8\xb3\f=\xf3\xcc3#\x97.]r\xe3k\xaf\xbd>\xa2\xa8\xa8\x88H$B\u01ce\x1d\x195j\xa4\x1a3f\x8c\xbf[\xb7\xae\x04\x02\t\xf1g;XV\xf4hIV\xfd\xa7\xab\x92P\x17\x86q\x1b&\xbb\xab\x1fc\xb10B\xb8\xae=##\x93\xa1C3\x192d\xb0\xbe{\xf7n\xfd\xf3\u03e72m\xda\x17j\xed\xdau\xe2\xc0\x81\x03\xcd\n\n\n\xee\x9d5kV\xffg\x9e\xf9\xcb\xf37\xdd\xf4\xfb9B\x88H\x93&\x19j\xe6\xcc\xe9\xfa\xa8Q\xa3\x9d\x1fs;\xcb\u028a\xed?\xdf\xf3'\xf2\xf2\xda,_\xb4h\xc1\x05EE\a\xfe\xb6d\u0252\x8cp8\xecD\xa3\x11\u0772\x1dP\x8aE\xaf>\x04\xbaA\x9f\x89w \x83\xc9D\xa2&8\x02\xa4[tK\b04\x81\xae\xc5\xc3N\xc2]d\xe4\xd8\x0e\xe9m:\u04e2\xd7\x10\x8e|6\x89}\xab\x16\xd0\xf2\xa4!4m\xd3\t\u06cc`\xa1\x10\x8e\u0110\x12\u01f2\t5iJ0\u0794Z\u0157\xf7G\xa31\x8e\x1c)\a\xa0\xbc\xbc\x9c\x993\xa72j\xd4\xd9\xde\x17\xc2\xc3\x13\xf3\x9f\x1b;vlc\xc0\x80\x93\u0168Q\xa7;\x00K\x96|3\xec\xe6\x9b\u007f\u007f\xc7\xe2\xc5K\x86o\u0672\x95H$\xc2\t'td\xec\u0631j\u0738\vD\xbbv\x1d\xc5\xd1\x10J\f\xc7q\xfe'>\x87R\x8aX,V\u007f21\f\x1f\xad[\xb7\xe3\xe6\x9bo\xe5\xe2\x8b\u01cb\xa9S\xa71y\xf2dV\xacX\xc9\xee\xdd{F\x14\x16\xee\x1dQX\xb8w\xee\u0085\xf3\x1e\x1d2d\u063cQ\xa3F;-Z\u42af\xbf^\xa8\xf2\xf3[\xff(\u06d4\x9e\xdeL-[\xb6X\xf4\xeb7P\r\x1at\u02ac7\xdex\xedV\u04cc\xbd\xb5r\xe5J\xddql\th\x96e\x03\xb0\xe8\xa5\xfb@)\xfa_u\x17\xaaq\x1a\x91#%8\xb6\u0114\uef02O\x13\x04\r\x1d]S\xf5\u92a6i\xa2\x05\x13\xc9:i\b;\x96\u03a6\xa2p\x1b%\x9bV\u04fcCg\x94\xa90\xa5\xc4q$\xd2qP\xd2\u0097\x90T\xbfx(\xfe\x12H\xa5\x88F#\x0e@ff3O\xc8=~\xb6\xfc*c\x84EE{\x99:u\x1a7\xde\xf8\u007f\r\xc50\xf9\xa6\x9b~{\xe5\xb6m\xdb\xfe\xbca\u00c6\u48a2\xfd\xe4\xe66g\xfc\xf8\xf1L\x980\x9e\u039d\xbb\u015d\xb0uL~\xf8\xff\xf4\xc1\x15\"\x9e\xf6\xe8\a\xe0\xc0\x81}|\xfc\xf1d\xde~\xfb\x1d\u05ad\xfb\x8e\xec\xec,\xbat\xe9R\u0561C\x87??\xfd\xf43o\b!\xaa\xea\x9e\xfb\xf6\u06d38\xe5\x94Sh\xd1\"\xff\xdf\u078e\xab\xae\xba\x92\xd7_\u007f\x03\x80G\x1ey\u8669S\xa7\u0774e\xcb\x16,\xcbR\xa6i\t+\x9e\x83\x0e\xd0\u007f\xe2\xad\xf4\x9dx;\x18A\xc2e\xc5\u0636\x8d\xe9Hl\x05\xba&\xf0k\xe0\xd3\xdc@\x8a\xa3\x14\xd2\x1f\xa2\xb6\xb2\x825o?\xce\ue15f\x93?`\x14C\xae\xb9\x93\xa4\xd4tjL\x1bt?\xc1\x80ABR\x12\xb6\x19e\xca\xed\x17\xb3s\xe9\x1c4]\xc7\b\x84\x9c\u071cl}\xf4\xa8\x11\x9f>\xf7\xdc\xf3\x13\x84\x10\x91\xc2\xc2]\xa2e\xcbV\xde$\xa8\xc7\u03ce_\xe5r\xfeo\xbeY|\x8c\x90/^\xbc\xa8\xef\x84\t\x17\u007f5s\xe6\u033f\u031a5;\xf9\u0421b\u018c9\x87\xd7^{\x95G\x1f}\x84\u039d\xbba\xdb1b\xb1p}\xbc\xfa\xe7\x80R\n\u06f6\xe3\xdb\x1d%;;\x97\xdf\xfd\xee\x0f\xbc\xf1\xc6k\xdcp\xc3\xf5\xc4b&s\xe6\xccM\x9e1c\xc6_.\xbf\xfc\xb2\xaf\u05ac)\xe8[\xf7\xdc\xcb.\xbb\x82\xa5K\x97\xfd(\xdb1n\xdc\xd16sw\xde\xf9\xc7\xdf\xf5\xee\xddkR\u02d6y\x18\x86!\x02\x01\xbf\xf2\xfb\xfd\xf5s\x03K\xdf|\x82yO\xddL\xf8\xf0A\x92\u049a\x12\xf0\x1b\x18\x9a\xc0\xd0\x04R*\xa2\x0e\x84-E\xad%\x89\xd8\xeed\xaa/)\x85f\x9d\xfbb\x84\x12(\u0771\x9e\x8a};\t\x04\x02\b\xe9\xa0\xe2\xee\u0736\x1d@\xa05\x98\x83\xd04!\"\x91(MR\x9bt\x06\xda\x03\u0636\ud578\xf0\xf0\xc4\xfc\xe7\xc0\xa2E\v\xc5E\x17\x8d\xaf\xbf\xff\xf8\xe3\x8f]\u007f\xd7]w\u007f2k\u05ac\x81\u06f6m'??O>\xf2\xc8\u00fc\xfc\xf2\x8b\x9cz\xeai8\x8eM$R\xf3?\x13N\xf9W\x91R\x12\x8d\xd6b\x9a\x11z\xf4\xe8\xc5\x13O<\u018b/\xbe@\x9f>}\xe4\xb6m\u06d81c\xc6\xc0[n\xb9e\xf2s\xcf=\xf3\x1b\xa5T\xc0\x15\xe1\xf1\u03181\xfd\xdf~\xef\xe1\xc3Oe\u0294\xc9\xf5\xf7\x9f}\xf6\xafw\x9e}\xf6Y\x85\xb9\xb9-\x10B\b\xbf\u07e7|>_\xfd<\xc0\xba\xa9o3\xeb\xa1\xeb)\u07bc\x8a`\xe3&\x84BA\fA\xbc@W}).\xb7\xe2\xa2\x02#\x10\"\xa5U'\x92\x9a\xe5R{\xb8\x98\x8a};\xb0L\xab\xfe\xaf\xed\xf8D\xaa\xb2-l\xdb>\xbaO\x14*\x10\nRU]\xb5\x1d\xd8\x1d\x0fQy\xae\xdc\xe3g\u026f*f>o\xdelm\u0420!2\xeeZ\x1b\xdd~\xfb\xad/\u007f\xf4\xd1\xc7\xe3\v\n\n\x00\x9cQ\xa3N\x15w\xdcq\x9b6x\xf0\x10\xc0\xcd\xe9\xd64\xad^d~\xf615!\x90R\x12\x0eW\x13\f\x069\xff\xfc\v\xe8\u05ad\x8b\xf6\xdcs\xcf\u02f7\xdf~G\u035b\xb7\xa0yuu\xcds\a\x0e\x1c\x1c\xa0\x94\xbaB\b\x11>\xed\xb4\xd1,]\xbaX\xeb\xdf\u007f\xe0\xbfu9r\xee\xb9\xe7S^^Fjj:B\x88\x92}\xfb\xf6^\\YY\xf5\xf1\xb4i\u04f2\x8b\x8b\x0f\t\xbf\u07e7\x84\x10\u00b2m\xa4\xe3\xb0{\xd6vD[\x00\x00 \x00IDAT\xf9\\j\x8a\x8b\xe8?\xf16\xf2\xfa\x8e !1\x89\xda\xdaZ\x1c\xe9\x80\x10\bM\u00f6lJ6\xad\xc0\xb1LR\xf3;\x92\u07be\a\x15\x85\xdb(\u07f7\x93hM9\x9a?\x19G)lGaK\x90\x910\x8e\x19\xab\xdf\x17\x80\n\x85B\x1c8pp\xa3\x10\xa223;K\xcb\xcd\u0353\x9e,xx\xce\xfc\u007f\x98\xd7_\u007fM\x1b6l\xa4\x04X\xb1bY\x9f\xf1\xe3/\x9a\xf5\u99df\x8d/(( 11Q\xddv\xdb-\xda\xdf\xfe\xf6\xb66x\xf0P\x00\xa2\xd1\b\xba\xae\xf3\xdf,X\xf5S\t\xba\xae\ub626\x89\u3634k\u05d1\xe7\x9e{Z{\xfa\u99f4\xbc\xbc\x96j\xe5\xcaUL\x992\xe5\xc2\xf3\xce;\xf7\xf3\x82\x82\xd5]\x00\xfa\xf7\x1f(?\xf8\xe0\xbd\u007f{\xac\xa4\xa6\xa63c\xc6W\x1a@nn\x8b%c\u01desf\x8f\x1e=\xf64i\x92\x86\x10B\x05\x02~\xfc>_}6P\xe9\xee-\xcc|\xf4&\x96\xbf\xf98\xe1\xb2\xfd$%7\xc6\xef\xf3#\xa4D\xd7t\x90\x0e\aV\xcfc\xcd[\x0f\xb3o\xe9t\x1ae\xb6@\xf7\xfb).\xdcAuy9\x9a\u03c7\x94\n)\x1d,\x05U\xe5e\xc4j\xea\x9aN\v\xea\u02a6\u06cek\xd7'\x8c\x1f\x1f\xb8\xf3\xce;|g\x9ey\x86~\xe3\x8d\xd7\ubbfe\xfa\xb2>g\xce,}\u04e6\rZ\xc3u\x02\x1e\x1e\x9e3\xff/\xf1\xc8#\x0f\x8b\xab\xae\xbaZ\x02|\xfc\xf1G\xe7>\xf1\u0113o-X\xb00\xe9\xf0\xe1\xc34o\x9e\xa3\xee\xb8\xe3vq\xe3\x8d7\x00:\xb6\x1dsK\xa8\x8a_\xf6\u0730\x10n\xd7\x1f\xc7q\xf0\xfbC\\u\xd55\"77\x97\xbb\xef\xbeG\xae^]\xa0UTT\x8e\b\x85\x9e^2}\xfa\x97\x13G\x8f>c\xcaE\x17],\x9f}\xf6\x19q\xd3M\xbf\xfb\xb7T\xed\xb4\xd3N\x97\xef\xbf\xff\x9e\x18?\xfeb5x\xf0)\u07fe\xfa\xea+cm\u06da\xf2\xed\xb7k\xf2\xcb\u02cfH\xc3\u0405\x10BX\x96\x1b\x12\x89\x85\xabY\xf5\xc1\xf3\x14o]G\xb7\xb1\u05d0\u0575?\xbe\xe4T\xccH-\t\t\x89d\xb4\xeeD\xe1\u0499\xec\x98\xfd1M\xf2;\xe0\v%RUz\x90\x8a\xcaJ\x82\xb9\x06J\xc4\xdcu\xb4B\x10\xa9\xac \x16\xae\x89\u007f~\x90\xd2\xc1\xef\xf7\xd3(\xa9Q-\xc0\x93O>\x15\xf9G\xdb=p\xe0@\xfdO\u007f\xba\x1b\u01d1j\xf8\xf0a\xea\xe4\x93\a(\xc3\b\xb2g\xcfN\xf2\xf2Z{J\xe2\xf1\xdf\xffN\xff\x92?\xdc\xef\u007f\u007f\x13\x8e\xe3\x88\xe7\x9e{^\x01<\xff\xfc_\xc7N\x9b6\xed\u0765K\x97\x85jjjh\u06f6\xadz\xfc\xf1G\xc59\xe7\x8c\x05\xc04#\xfcZ\x1dX \x10\x044\u05ae\xfd\x96;\xee\xb8S\u035a5[\xa4\xa44f\u0420A\x911c\xc6L\x988\xf1\x8aO\x01n\xbd\xf5\x16\xe1\xf7\xfb\xd5C\x0f=\xfcO\xbf\u01de=;\xf9\xeb__\u2a67\x9e\xe4\uaaef\xe6\xb5\xd7^\x03\xe0\xbd\xf7\xde\xed>u\xea\xd4OV\xacX\u046a\xa4\xa4\x04!4%\xa5\x8c/,r\x87\xa9t,\x92\x9ad\xd2\xfe\x941\xb4\x1dq.\xa9y\x1d\x11\x02\xecX\x94\xed\xf3\xa6\xb0i\xc6\xfbD*\x8f\xa0l\x1b4A\xff\xdf<F\x8b>\u00d0\xb1(J@Bj\x06{\x17\u007f\xc1\xdc\xc7~Cmy\x19\x86\u03cf\x12\x9a\u04f6Mk}\xe4\xf0\xa1\u04c7\x0f\x1b\xf6F\xd1\xfe\xfdM5M\v\xef\u077bw\xe5\xc9'\x0f\x8c\x0e\x1f>\xd40\x8c\xa0\x01\xec\xfd\x9e\x92\xc3\xc7T\xba\\\xb6l\t\xfd\xfa\r\xf0\x14\xc5\xc3\x13\xf3\x9f\x82\x1bn\xb8^\xbc\xf8\xe2K\n\xe0\xc5\x17\xffz\xfaG\x1fM\x9e\xbcr\xe5\xaaP$\x12\xa1[\xb7\xae<\xf9\xe4\x13\f\x1f>\x12)M\xea\xf2\x9d\u007f\u0543A\b\xfc\xfe\x10\x85\x85\xbb\xb8\xf3\xce?\xf2\xe1\x87\x1f\x92\x90\x90\xc0\xc0\x81\x03#\xe3\u018d;\u007f\xe2\xc4+\xbe\x02\xb8\xed\xb6[\xc5\xe3\x8f?\xf1\x0f\xcfz\xdf~\xbb\x9a\x1e=z\xfeS\xef]SS\xde\xea\xce;\xef\x992s\xe6\xac\x13\x8b\x8a\x8a\\\xb1\x94\x12\xa9\x14\b\x1d!t\x94t\x90\xb6EZ\xab\x8et\x18u\x11m\x06\x9fIJ\xb3\xe68\xb10;W-b\xeb\xbcO)\u07b8\x12'\x16\xa5\xf7\xb5\xf7\x91?\xe8\f\x84\xb4\xd1\x04$\xa5g\xb1\xe1\xb3W\x99\xf7\xd4\x1fp,\v\xe16\x0e\xc5\xd05\u0661C\a-'';~\xa5\"),,\\\u05e8QR$33\u04d7\x96\x96\xee\x03\xb9\xc3\xe7\xf3\xefh\u07fe}E\xf3\xe6\xcd\x17]r\u0265\x87\x85\x10[\xbe\xefs\xfc\xf1\x8fw1x\xf0 F\x8e\x1c\u5a4b\x87'\xe6?\x06o\xbe9IL\x9cxE\x9d\x90\x8f\xfe\xf0\u00cf\xdf[\xb5jUJ$\x12\xa5W\xaf\x9e<\xfb\xec\xd3\xf4\xeb7\x10\u02ca\xe28\xce/>\xac\xf2Cq\xcb\xf5&RZZ\xcc\x1f\xffx7\x93&M\xc2\xef\xf73`\xc0\x80\x8aK/\xbd\xf4\xe2K/\xbdl:\xc0G\x1f} .\xbc\xf0\xa2c\x04\xfd\xb3\u03e60f\u0339?\xe4=\xda\xee\u07bd\xa3\xe3\xea\xd5\x05r\xf7\xee\u0772\xb2\xb2Jj\x9aV2l\xd8\xd0\xd3\u05ee]w\xff\xfb\xef\xbf\u03d6-[\xb0m\xc7\xcd\"\x12\x1aB\xe8\b\xcd\x00\x01v4\x8c\x11\b\u0462\xe7`N\x18y.-N:\x05\xbdq\x06\xbbW-`\xc5\xcb\u007f\xa6r\xdf\x0ez]s/\xad\x86\x8dE\x03\f\rR\xd23Y\xfc\u04bd,\x99\xf4\x88\x1bc\xf9\xfe+0\xe5\x9e\xd3D})\x06M\xd3HJJ\"++\x93\x94\x94\x14,\xcb\u079b\x99\x99Y\x99\x9f\x9fw 77w\xe6%\x97L\xf8\xb2Y\xb3\xac\x1d\xff\xaf\xfd\xe9\x8d-\x0fO\xcc\xffEV\xae\\&z\xf7\xee\xa7\\\xd1\xf9p\xf4\xcb/\xbf\xf2\xde\xe2\u014bS,\u02e2w\xef\u07bc\xf0\xc2s\xf4\xec\xd9\a\u04cc\xd4\xd78\xf18\x8a\x94\x92P(\x89\x8a\x8a\xc3\xdcq\u01dd\xbc\xfa\xeak\xe8\xba\xce\u0211\xa7V\xfc\xfe\xf7\xbf\xbbx\u0108\x91\xd3\x01V\xaf^!z\xf6\xec\xf3\x0f\x1d\xfa\xaaU+\xfa}\xf6\xd9\xe7\xadJKK\xba$$$\x0e\u07bd{O\xb8\xac\xacL$$$\xe4\x80\u02a8\xaa\xaaR\xb1XL\u0555\xfe5\f\xa3\xa6}\xfb\xf6\xd9EEE\xbe-[\xb6P[S\x8b\xed\xb8\x95\x0fu\u0747\xa6\x1b\bMC\xd3t\x1c\xdb\u010eFH\xca\xc8&\xb7\xfb@:\x9c~\t\xca\b\xb0\xf4\x85\xbb(\u07fd\x99>\xd7\xddG\x9ba\xe7!P\x18~\x1f2Z\xcb\xfc\xc7\u007f\xcf\xf6E\xd3\x10\x9a\x86j\xb0V\xa0\xaeDB\x03\x01\x96\xc7=\xae\x1c\u01d1\x80\x1e\n\x055\x9f\xcfOVV&\t\t\x894j\x94\xb4\xbbC\x87\x0e\xbb\xf3\xf2\xf2>\x1f>|\xd8w\xbd{\xf7\xfd\xfa\xfb\xf6\xc5\xff\xfd\xdf\r\\{\xed5t\xe9r\xa27\xc0<<1\xff!\xbc\xf3\xce[\xe2\xd2K/\aP\xdf~[0\xfc\xfe\xfb\xef\x9f2}\xfa\x8cd\xd34\x9d\xee\u077b\ubbfc\xf2\"\xbdz\xf5\xf5\x84\xfc\a\nzyy\x197\xdf|+o\xbe\xf9\x96\x13\b\x04\xf41c\xc6T\xdd{\xef=c:v\xec4?\x18\fj\xd1h\xb4auI\u07c2\x05\xf3:~\xf8\xe1G\x83\x0e\x1e<x\xd9\xee\xdd{:\x16\x17\x17k\x9a&B\xc1`\x90h4\x86e\xd9\xc4bQ\x1c\xc7F\b\xed8!u\x17\x01%&&*P\"\x1c\x8e\xb8\xcd=\x00\xc3\xf0c\x18>4]\a\xb4x\x98\x04\xacH\x18P\xa4\xb4hKbz6\xe5{\xb7a\x9b\x11\x86\xde\xfa,\xad\x87\x9c\x8dm\x9a\x04\x92\x1aQ\xba\xa5\x80\x19\u007f\xbe\x92\x92m\xeb\xfeN\u033f/\xdcTGC\x87\x1e/ \xe6(\xa5\xb0,\xcb\xd14\xcd\u07e8Q\x12\xa9\xa9\xa9\x04\x83A'9\xb9QU\xabV\xad\xcb:v\uce22}\xfb\xb6S\x87\r\x1b\xb6*##\xb3\xf0\xf8\xd7_\xb6l\x89\xe8\xd7o\x80\x97\x1e\xe3\xe1\x89\xf9\x0f\xb9\xa4-**l\xf5\xdb\xdf\xfe~\xfa\xfc\xf9\xf3\xdbWTT\u0431c\a^\u007f\xfdU\xfa\xf7?\x19\xa5\xdc\x06\x10\x9e\x90\xff\xff\f\x8ex\f\xfd\u02112\xae\xbd\xf6:>\xf9d\niiM\x18:t\xe8\xc6\x17^x\xee\xf4\xa6M\xb3\v\xe3\xfb=\xf5\x81\a\xee;s\u04e6\u037f\u0674is\xcf\x1d;v \x84 \x1c\x0e7\x9cP\xae\xaf\u007fX\x97\x1e\xf9}\xf9\xfbu\xc7\xd00\x8c\xfa\x15\xac\xb6m\xbb\xee\xdc\xf0\xa3\xeb\x06\x9a\xa6\xe3\u0583\xaf;\xf1\xd8H\xcbD\xd3\f4\x9f\x8f\xa4\xa6\xd9\xf4\xbb\xf2.\xd2\xdbt&RSE \x94\xc8\xc1\x8d\xabX\xfa\xda\x03T\x97\x1e\xa8\xaf\xeaX\xf7\x15h8\f\xea\xab4\xba\xa7\xa7\xef\x1d#\rJ\xfc\"\xa5T\x80\xd0u\xb7\xcd_rr2\x81@\x80\xb4\xb4&t\xea\xd4\xc9NII\xf9\xa8m\u06f6_\xe6\xe7\xb7Z}\xd6Yg\x954,\x99\u0423G\x0f\xe3\xba\xeb\xaea\xe4\xc8\x11N^^kO\xdc=<1\xff\x9e/[\xfa\x84\t\xe3\u07de7o\xfe\xe8C\x87\x8a\xc9\xc9\xc9\xe1\xf9\xe7\x9f\xc3\xcdZq\x88F\xa3\x9e\x90\xff@4M\xc3\xe7\v\xb2c\xc7V\xae\xb9\xe6:\x16,XHNN6#G\x8e\xfcj\u04a47\xcf\x02|\xbf\xfb\xddM\x9f~\xf3\u0362\xd1\x1b7n\"\x163\x8f9\x14\xc4c\xd0J\xb9\xa1\xea\xba\xfd\xfe}9\xfc\r\ufeee\xdd}\t\xdbvk\xe1h\xba\x0fM3\x10\x9a~\x8c\x007\x8c\x81\vM#\u0538\t\x89M\x9ab[&V,\x82t\x1cb5\x95\x84+\u0290\x8e\xddP\xad\u007f2|>\x1f\xc1`\x90`0H\xeb\u05ad\xc9\xc9\xc9.MKK[\x92\x96\x96V\x90\x92\xd2x\xfem\xb7\xddQ$\x84\xd8[\xf7\xf7o\xbd\xf5\x866r\xe4\xa92;\xbb9\xf3\xe7\xcfe\xe8\xd0\xe1\xde\xe0\xf3\xf8\xf5\x89\xf9\x8c\x19_\xb1p\xe1\xd7\xe2\xb1\xc7\x1eW\x00\xf7\xdcs\xf7\vS\xa6|z\u00e6M\x9bHHHP\x8f?\xfe\x98pk\xb1(\xa2\u0470'\xe4\xff\xe4\u054ea\x18\x18F\x80\xe5\u02d7r\xf5\xd5\u05e8\r\x1b6\x8aN\x9dN\xe0\x8a+&>\x9e\x94\x94\xd4\xf4\xd9g\x9f\xbb|\xf3\xe6-\xff0\xad\xb3n\u007f7lX}\xb4\xd6\xfb?h\u05a1\x14R)\xb7T\xadRnHL\xd3\u30ae\xd7\u05c9o(\xca\xf1P7B)\xecX\fG\xd9?H\xb2u@\xd3\x05\xb6\xa3~\x12\x897\f\x03\xbf\xdfG(\x14\"33\x93\x9c\x9c\x1c\u0574i\xd3\xe5\x19\x19\xe9S\u018e\x1d\xb3\xec\u44c7,=\xfe93g~\u0168Q\xa7{\x03\xd0\xe3\xd7%\xe67\xdex\x03/\xbc\xf0\"\x00\xaf\xbe\xfa\u0288\x17_|q\u06bau\xdf\x05\x95R\xfc\xe1\x0f\xbf\xe7\xd1G\x1f\xc6\xe7\xf3y\x8e\xfc\xdf\x10tM\xd3\xf0\xfbC|\xf8\xe1\xfb\xfc\xf6\xb77QZZF\x9f>\xbdcyy-\x03\v\x16,\xa4\xa4\xa4L\x89x\xe7\x88\xe3E\xfd\x18\xc7]\xe7\xa4\x1b\x8a|\x03\x1b\x0f\n%]!\xff\u01c36^b\xa1>\xde\xee\xde\xfc\t\x894JoF()\x19_(\x01_ \x88\xe1\xf3\xe1\xf3\x19\x84\xfc\x06\x86\xc0\x9dh\xd5\x04\x89\x8d\x92\xc9h\u0782\xe4\x8cl\x84\xe1\aM#\x1251-\x13G\xbaW\x03V\xa4\x96HM5\xb5U\x15T\x94\x1e\xa2d\xefN\x8e\xec\xdbE\xac\xb6\xa6~\xfb\xea?O\x83\xab\x83\xff\xd7IM\b\xd04\x9dF\x8d\x1a\u047auk\xd2\xd3\u04cb\xf3\xf3\xf3\xbe>\xf7\u0731\x8b\x86\r\x1b\xf1\x8a\x10\xa2>G\xf6\x92K&0b\xc4p\xe2s@\x1e\x1e\xbfl1\u007f\xe2\x89\u01f8\xf5\xd6\xdb\xeb\xbeDY\xa7\x9f~\xfa\xeco\xbe\xf9\xa6suu5g\x9cq:o\xbc\xf1\x1aM\x9bf\x11\x0eW\xff\xa4M#~\xe9H\xa9\xf0\xfb}\b!\xb8\xff\xfe\ax\xf8\xe1G\xd1u\x9d\xe4\xe4d\x94\x92\xaa\xb2\xb2\xaa\xbe{\xd0O\xb1\xf0J\xd7\r\x8c@\x00_ \xc1m\xd4l\u06d8\xd1\bB\bB\u0269d\xb4\xea@\xeb\x01\xa7\u04b4M\x17t\xbf\x1f_ \x80?\x10\xc4\xef\xf7\x91\x14\xf4\x91\x92\xe0\xc3\xf0\xb9\xd5\x195\x01\x9a/@(9\x15\x15\x00G\x82\xe5\x80%\x8f\x9eT\x1c\a\xa4\x05\x96\x19\xc32\xa3\x985U\u0516\x1f\xa6\xa4p'\xfb\xb7\xac\xa1\xe8\xbb\x15\x14\xef\xdcL\xb4\xaa\x1c+v\xb41\x89\xe6\xaau\xbc\u035d\x02)\xff\xce\xed\xc7E]*\x05~\xbfO\xcb\xcf\xcf'



var dashboardDockerfile = `
FROM mhart/alpine-node:latest

RUN \
	npm install connect serve-static && \
	\
	echo 'var connect = require("connect");'                                > server.js && \
	echo 'var serveStatic = require("serve-static");'                      >> server.js && \
	echo 'connect().use(serveStatic("/dashboard")).listen(80, function(){' >> server.js && \
	echo '    console.log("Server running on 80...");'                     >> server.js && \
	echo '});'                                                             >> server.js

ADD {{.Network}}.json /dashboard/{{.Network}}.json
ADD {{.Network}}-cpp.json /dashboard/{{.Network}}-cpp.json
ADD {{.Network}}-harmony.json /dashboard/{{.Network}}-harmony.json
ADD {{.Network}}-parity.json /dashboard/{{.Network}}-parity.json
ADD {{.Network}}-python.json /dashboard/{{.Network}}-python.json
ADD index.html /dashboard/index.html
ADD puppeth.png /dashboard/puppeth.png

EXPOSE 80

CMD ["node", "/server.js"]
`



var dashboardComposefile = `
version: '2'
services:
  dashboard:
    build: .
    image: {{.Network}}/dashboard{{if not .VHost}}
    ports:
      - "{{.Port}}:80"{{end}}
    environment:
      - ETHSTATS_PAGE={{.EthstatsPage}}
      - EXPLORER_PAGE={{.ExplorerPage}}
      - WALLET_PAGE={{.WalletPage}}
      - FAUCET_PAGE={{.FaucetPage}}{{if .VHost}}
      - VIRTUAL_HOST={{.VHost}}{{end}}
    logging:
      driver: "json-file"
      options:
        max-size: "1m"
        max-file: "10"
    restart: always
`




func deployDashboard(client *sshClient, network string, conf *config, config *dashboardInfos, nocache bool) ([]byte, error) {
	
	workdir := fmt.Sprintf("%d", rand.Int63())
	files := make(map[string][]byte)

	dockerfile := new(bytes.Buffer)
	template.Must(template.New("").Parse(dashboardDockerfile)).Execute(dockerfile, map[string]interface{}{
		"Network": network,
	})
	files[filepath.Join(workdir, "Dockerfile")] = dockerfile.Bytes()

	composefile := new(bytes.Buffer)
	template.Must(template.New("").Parse(dashboardComposefile)).Execute(composefile, map[string]interface{}{
		"Network":      network,
		"Port":         config.port,
		"VHost":        config.host,
		"EthstatsPage": config.ethstats,
		"ExplorerPage": config.explorer,
		"WalletPage":   config.wallet,
		"FaucetPage":   config.faucet,
	})
	files[filepath.Join(workdir, "docker-compose.yaml")] = composefile.Bytes()

	statsLogin := fmt.Sprintf("yournode:%s", conf.ethstats)
	if !config.trusted {
		statsLogin = ""
	}
	indexfile := new(bytes.Buffer)
	bootCpp := make([]string, len(conf.bootnodes))
	for i, boot := range conf.bootnodes {
		bootCpp[i] = "required:" + strings.TrimPrefix(boot, "enode:
	}
	bootHarmony := make([]string, len(conf.bootnodes))
	for i, boot := range conf.bootnodes {
		bootHarmony[i] = fmt.Sprintf("-Dpeer.active.%d.url=%s", i, boot)
	}
	bootPython := make([]string, len(conf.bootnodes))
	for i, boot := range conf.bootnodes {
		bootPython[i] = "'" + boot + "'"
	}
	template.Must(template.New("").Parse(dashboardContent)).Execute(indexfile, map[string]interface{}{
		"Network":           network,
		"NetworkID":         conf.Genesis.Config.ChainID,
		"NetworkTitle":      strings.Title(network),
		"EthstatsPage":      config.ethstats,
		"ExplorerPage":      config.explorer,
		"WalletPage":        config.wallet,
		"FaucetPage":        config.faucet,
		"GethGenesis":       network + ".json",
		"Bootnodes":         conf.bootnodes,
		"BootnodesFlat":     strings.Join(conf.bootnodes, ","),
		"Ethstats":          statsLogin,
		"Ethash":            conf.Genesis.Config.Ethash != nil,
		"CppGenesis":        network + "-cpp.json",
		"CppBootnodes":      strings.Join(bootCpp, " "),
		"HarmonyGenesis":    network + "-harmony.json",
		"HarmonyBootnodes":  strings.Join(bootHarmony, " "),
		"ParityGenesis":     network + "-parity.json",
		"PythonGenesis":     network + "-python.json",
		"PythonBootnodes":   strings.Join(bootPython, ","),
		"Homestead":         conf.Genesis.Config.HomesteadBlock,
		"Tangerine":         conf.Genesis.Config.EIP150Block,
		"Spurious":          conf.Genesis.Config.EIP155Block,
		"Byzantium":         conf.Genesis.Config.ByzantiumBlock,
		"Constantinople":    conf.Genesis.Config.ConstantinopleBlock,
		"ConstantinopleFix": conf.Genesis.Config.PetersburgBlock,
	})
	files[filepath.Join(workdir, "index.html")] = indexfile.Bytes()

	
	genesis, _ := conf.Genesis.MarshalJSON()
	files[filepath.Join(workdir, network+".json")] = genesis

	if conf.Genesis.Config.Ethash != nil {
		cppSpec, err := newAlethGenesisSpec(network, conf.Genesis)
		if err != nil {
			return nil, err
		}
		cppSpecJSON, _ := json.Marshal(cppSpec)
		files[filepath.Join(workdir, network+"-cpp.json")] = cppSpecJSON

		harmonySpecJSON, _ := conf.Genesis.MarshalJSON()
		files[filepath.Join(workdir, network+"-harmony.json")] = harmonySpecJSON

		paritySpec, err := newParityChainSpec(network, conf.Genesis, conf.bootnodes)
		if err != nil {
			return nil, err
		}
		paritySpecJSON, _ := json.Marshal(paritySpec)
		files[filepath.Join(workdir, network+"-parity.json")] = paritySpecJSON

		pyethSpec, err := newPyEthereumGenesisSpec(network, conf.Genesis)
		if err != nil {
			return nil, err
		}
		pyethSpecJSON, _ := json.Marshal(pyethSpec)
		files[filepath.Join(workdir, network+"-python.json")] = pyethSpecJSON
	} else {
		for _, client := range []string{"cpp", "harmony", "parity", "python"} {
			files[filepath.Join(workdir, network+"-"+client+".json")] = []byte{}
		}
	}
	files[filepath.Join(workdir, "puppeth.png")] = dashboardMascot

	
	if out, err := client.Upload(files); err != nil {
		return out, err
	}
	defer client.Run("rm -rf " + workdir)

	
	if nocache {
		return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s build --pull --no-cache && docker-compose -p %s up -d --force-recreate --timeout 60", workdir, network, network))
	}
	return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s up -d --build --force-recreate --timeout 60", workdir, network))
}



type dashboardInfos struct {
	host    string
	port    int
	trusted bool

	ethstats string
	explorer string
	wallet   string
	faucet   string
}



func (info *dashboardInfos) Report() map[string]string {
	return map[string]string{
		"Website address":       info.host,
		"Website listener port": strconv.Itoa(info.port),
		"Ethstats service":      info.ethstats,
		"Explorer service":      info.explorer,
		"Wallet service":        info.wallet,
		"Faucet service":        info.faucet,
	}
}



func checkDashboard(client *sshClient, network string) (*dashboardInfos, error) {
	
	infos, err := inspectContainer(client, fmt.Sprintf("%s_dashboard_1", network))
	if err != nil {
		return nil, err
	}
	if !infos.running {
		return nil, ErrServiceOffline
	}
	
	port := infos.portmap["80/tcp"]
	if port == 0 {
		if proxy, _ := checkNginx(client, network); proxy != nil {
			port = proxy.port
		}
	}
	if port == 0 {
		return nil, ErrNotExposed
	}
	
	host := infos.envvars["VIRTUAL_HOST"]
	if host == "" {
		host = client.server
	}
	
	if err = checkPort(host, port); err != nil {
		log.Warn("Dashboard service seems unreachable", "server", host, "port", port, "err", err)
	}
	
	return &dashboardInfos{
		host:     host,
		port:     port,
		ethstats: infos.envvars["ETHSTATS_PAGE"],
		explorer: infos.envvars["EXPLORER_PAGE"],
		wallet:   infos.envvars["WALLET_PAGE"],
		faucet:   infos.envvars["FAUCET_PAGE"],
	}, nil
}
