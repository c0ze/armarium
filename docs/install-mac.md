# Install Armarium on a Mac

Armarium serves the books and comics on your Mac to your browser, your phone
and your tablet. It runs quietly in the menu bar while you're logged in. You
don't need Terminal or a configuration file.

## What you need

- A Mac with macOS 13 Ventura or later (Apple silicon or Intel).
- The folder that holds your books, comics or Calibre library. It can be on an
  external drive or a network share.
- To read on a phone or tablet: the **Skrivist Books** app
  ([Google Play](https://play.google.com/store/apps/details?id=ist.skriv.books);
  the App Store version is in review) or the Skrivist web readers.

## 1. Download

Download **Armarium-X.Y.Z-macOS.dmg** from
[armarium.skriv.ist](https://armarium.skriv.ist/) or the
[latest release](https://github.com/c0ze/armarium/releases/latest).

## 2. Install

1. Open the downloaded `.dmg` file.
2. Drag **Armarium** onto the **Applications** folder in the window that opens.
3. Eject the Armarium disk in the Finder sidebar.

## 3. Open it

1. Open **Armarium** from your Applications folder.
2. macOS asks whether you're sure you want to open an app downloaded from the
   Internet. Click **Open**. Armarium is signed by Gand and checked by Apple.
3. No window opens. A small book icon appears in the menu bar at the top right of
   the screen, and your browser opens the setup page.

If the browser didn't open, click the book icon in the menu bar and choose
**Open Armarium**.

## 4. Set it up

The setup page asks four things:

1. **The owner password.** You use it to log in and to change settings. Choose
   one with at least 8 characters.

   ![The setup page asking for the owner password](images/install/setup-password.png)
2. **Where your books are.** Click **Choose folder…** and pick a folder. Add as
   many as you like. Armarium works out whether each one holds books, comics or a
   Calibre library; you can change that with the Books/Comics switch. Armarium
   only reads these folders. It never changes, moves or deletes your files.

   ![Two folders added: Books and Comics](images/install/setup-libraries.png)
   - If the folder is in Documents, Downloads, Desktop, an external drive or a
     network share, macOS asks whether Armarium may access it. Click **Allow**.
3. **Who can read.**
   - **This computer only**: your browser on this Mac, and the Skrivist apps for
     Mac and Windows.
   - **Devices at home**: phones and tablets on the same Wi-Fi as well.
   - **Also from anywhere, with Tailscale**: see
     [Read anywhere](#read-anywhere-with-tailscale) below.

   ![Who can read: this computer only, devices at home, or anywhere with Tailscale](images/install/setup-who-can-read.png)
4. **Start Armarium when you log in.** Leave this on, so your library is there
   whenever the Mac is.

Click **Start reading**. Armarium reads through your folders and shows your
library. A large library takes a few minutes the first time; you can start
reading meanwhile.

## 5. Read on your phone or tablet

1. In Armarium's Settings, **Who can read** must be **Devices at home**.
2. Install Skrivist Books on the phone. The phone must be on the same Wi-Fi as
   the Mac.
3. Click the book icon in the menu bar and choose **Pair a phone…**.
4. Pick the device, then click **Skrivist Books app (iPhone, iPad, Android)**. A
   QR code appears.
5. In Skrivist Books on the phone, tap **Add library**, then **Scan QR code**, and
   point it at the QR code.

The phone can read whenever the Mac is awake and Armarium is running. To keep the
Mac from sleeping while it's plugged in, look in System Settings → Displays →
Advanced (or Energy on a desktop Mac).

Your Mac's address on the home network can change, for example after the router
restarts. Armarium keeps up by itself, but a phone paired with the old address
can't find it. If that happens, pair the phone again. To keep the address the
same, reserve it for the Mac in your router's settings ("DHCP reservation").

### Read anywhere, with Tailscale

[Tailscale](https://tailscale.com/) is a free private network that connects
your own devices wherever they are. With it, your phone can read from the Mac
when you're away from home, over a secure HTTPS address, and the Skrivist web
readers (books.skriv.ist and comics.skriv.ist) can use it too.

1. Install Tailscale on the Mac and on the phone, and sign in to the same account
   on both.
2. In the Tailscale admin console, open **DNS** and turn on **HTTPS
   Certificates**.
3. In Armarium's Settings, tick **Also from anywhere, with Tailscale** and click
   **Save and restart**.
4. Pair the phone again from **Pair a phone…**. The QR code now carries the
   Tailscale address.

Armarium only adds itself at the root of your Mac's Tailscale address. If
something else already uses that address, Armarium leaves it alone and says so.

## Add more folders later

Click the book icon and choose **Settings…**. You can add folders, rename a
library (its reading progress stays), remove one, change who can read, or change
the password. **Save and restart** applies the changes; Armarium restarts in a
second or two.

Removing a library deletes its reading progress from Armarium. Your files stay
where they are.

## Update

1. Download the new `.dmg`.
2. Click the book icon and choose **Quit Armarium**.
3. Drag the new Armarium onto Applications and choose **Replace**.
4. Open Armarium again.

Your settings, libraries and reading progress are kept.

## Uninstall

1. Click the book icon and choose **Quit Armarium**.
2. Drag Armarium from Applications to the Bin. Its login item goes with it.

Your settings and reading progress stay in
`~/Library/Application Support/Armarium`, in case you install Armarium again.
To remove everything, delete that folder and `~/Library/Logs/Armarium` too. (In
the Finder, choose Go → Go to Folder… and paste the path.)

If you turned on Tailscale, also run this once in Terminal:

```bash
/Applications/Tailscale.app/Contents/MacOS/Tailscale serve --https=443 --set-path=/ off
```

## Forgot the password?

Open Terminal and run:

```bash
/Applications/Armarium.app/Contents/Helpers/armarium --config ~/Library/Application\ Support/Armarium/armarium.toml password
```

Type the new password twice. Browsers that were logged in have to log in again;
paired phones keep working.

## If something goes wrong

- **The menu says "Port 8580 is in use by another program".** Another program,
  perhaps another copy of Armarium, uses Armarium's port. Quit it, or choose
  another port in Settings.
- **The phone can't connect.** Check that the phone is on the same Wi-Fi, that
  Settings says **Devices at home**, and that the Mac is awake. If the Mac's
  firewall is on (System Settings → Network → Firewall), allow Armarium there.
- **More detail:** the book icon → **Show log folder** opens Armarium's log.
  Include it when you [report a problem](https://github.com/c0ze/armarium/issues).

## Where Armarium keeps its files

| What | Where |
|---|---|
| The app | `/Applications/Armarium.app` |
| Settings (`armarium.toml`) | `~/Library/Application Support/Armarium` |
| Library database and cover cache | `~/Library/Application Support/Armarium/data` |
| Log | `~/Library/Logs/Armarium` |

`armarium.toml` is written by Armarium. Quit Armarium before you edit it by hand.

## For Terminal users

The `armarium` command line is inside the app. To have it on your PATH:

```bash
sudo ln -s /Applications/Armarium.app/Contents/Helpers/armarium /usr/local/bin/armarium
```

Pass the desktop app's config to use its library, for example
`armarium --config ~/Library/Application\ Support/Armarium/armarium.toml token list`.
The command line still runs servers with a hand-written config too; see
[deploy.md](deploy.md).
