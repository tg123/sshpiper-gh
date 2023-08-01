# sshpiper github app

ssh with your github identity!
this is a plugin to [sshpiper](https://github.com/tg123/sshpiper)

## How to use

  1. Create a **PRIVATE** github repo and add `sshpiper.yaml` to it, you can find more example yaml at https://github.com/tg123/sshpiper-gh/blob/main/example/sshpiper.yaml
    
   ```
# yaml-language-server: $schema=https://raw.githubusercontent.com/tg123/sshpiper-gh/main/schema.json
upstreams:
  # pipe to example@github.com with password `fake`
  - host: github.com
     username: exmaple
     password: fake
   ```
   
   ![image](https://user-images.githubusercontent.com/170430/221841742-24ef6df2-39c9-4c11-a905-1add3719f436.png)


   2. Install [this App](https://github.com/apps/sshpiper) to the private repo

   ![image](https://user-images.githubusercontent.com/170430/221841450-1f396e6a-92d6-4505-b498-b9dddb98de24.png)


   3. `ssh sshpiper.com` and approve the connection with your browser (require github login)
   
   ```
   ssh sshpiper.com
please open https://sshpiper.com/pipe/09710885-cda5-41cf-8c73-e09617e07f01 with your browser to verify (timeout 1m)
```

## sshpiper.com Public Key

```
sshpiper.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICKZHEtpQZNUXgVlGLViAy7P0264kbFUDnQR4E+mylWM
```
