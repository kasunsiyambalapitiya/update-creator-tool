# Update Creator Tool

### Introduction

Update Creator Tool is a tool which would help to create new updates and validate the update zip file after any manual editing. This tool is written in GO language. One of the main advantages of writing this in GO is that we can compile the code directly to machine code so we don’t need something like JVM to run this tool. Another advantage is we can directly cross compile the code.

This tool mainly provides 2 functions.

1. Creating a new updates for both C4 and C5 based products
2. Validating an Update (only for C4 based products)

### Installation

First you need to install GO to compile and run this tool. You can find instructions to how to download and install the GO from the [Official Website](https://golang.org/doc/install). 

Then run the following command.

`go get -u github.com/wso2/update-creator-tool`

This will download and install the packages along with their dependencies. 

Then run `build.sh`. This will generate the executable files for various OS/Architecture combinations. These will be located at **/build/target/** directory. Extract the relevant zip file to your OS/Architecture. In the *bin* directory, you'll find the executable **wum-uc** file. 

**Note:** Add this path to your system path variables. Then you'll be able to call **wum-uc** tool from anywhere.

### Creating an update for a C4 based product

This is done using the `create` command. Place all of your updated files in a directory. If there are any configuration changes, instructions related to the update, you have to add them in a file called **instructions.txt**. Lets call this directory **UPDATE_LOCATION**. This directory should following files.

1. All of the updated files (binary, resource files)
2. update-descriptor.yaml
3. LICENSE.txt

Optional Files -

1. NOT_A_CONTRIBUTION.txt
2. instructions.txt

**Note:** You can generate the **update-descriptor.yaml** file using the **init** command as shown in the next section. You need to have the product(which you are creating the update for) distribution locally as well. This is used to compare files and create the proper file structure in the update zip.

Some samples for the **UPDATE_LOCATION** directory is shown below.

**Sample 1**
```bash
├── axis2_1.6.1.wso2v16.jar
├── instructions.txt
├── LICENSE.txt
├── NOT_A_CONTRIBUTION.txt
├── synapse-core_2.1.5.wso2v2.jar
└── update-descriptor.yaml
```

**Sample 2**
```bash
├── LICENSE.txt
├── NOT_A_CONTRIBUTION.txt
├── oauth2.war
└── update-descriptor.yaml
```

**Sample 3**
```bash
├── LICENSE.txt
├── NOT_A_CONTRIBUTION.txt
├── org.wso2.carbon.apimgt.hostobjects_5.0.3.jar
├── store
│   ├── modules
│   │   └── subscription
│   │       ├── list.jag
│   │       └── module.jag
│   └── site
│       └── blocks
│           └── subscription
│               └── subscription-list
│                   ├── ajax
│                   │   └── subscription-list.jag
│                   └── block.jag
└── update-descriptor.yaml

```

**Sample 4**
```bash
├── bin
│   └── tomcat-juli-7.0.69.jar
├── lib
│   └── endorsed
│       └── tomcat-annotations-api-7.0.69.jar
├── LICENSE.txt
├── org.wso2.carbon.tomcat_4.4.3.jar
├── tomcat_7.0.59.wso2v3.jar
├── tomcat-catalina-ha_7.0.59.wso2v1.jar
├── tomcat-el-api_7.0.59.wso2v1.jar
├── tomcat-jsp-api_7.0.59.wso2v1.jar
├── tomcat-servlet-api_7.0.59.wso2v1.jar
└── update-descriptor.yaml
```

#### init command

This command will generate the **update-descriptor.yaml** file. If no arguments are provided, this will init the current working directory. You can provide a directory path as an argument otherwise. If there is a README.txt in the old patch format in the initializing directory, this command will try to parse the necessary details from the README.txt file and use them to populate **update-descriptor.yaml** file. Otherwise, the fields will have a default value which you need to edit manually.

```bash
wum-uc init [<directory>]

<directory> - Directory which needs to be initialized. If this is not provided, current working directory will be initialized.
<flags> - Flags for the tool. Currently, supported flags are -d and -t which will print debug logs, trace logs.
```

**NOTE:** After running this command, don't forget to copy the **LICENSE.txt** from **<WUM-UC_HOME>/resources/LICENSE.txt** to the **UPDATE_LOCATION** directory if the update falls under EULA. If it is a security update, add the Apache License.

#### create command

This command will create a new update. As we mentioned earlier, don't forget to copy the correct **LICENSE.txt** file to the **UPDATE_LOCATION** directory before running this command.

```bash
wum-uc create <update_loc> <dist_loc> [<flags>]

<update_loc> - Location of the updated files.
<dist_loc> - Location of the distribution zip file.
<flags> - Flags for the tool. Currently, supported flags are -d and -t which will print debug logs, trace logs.
```

If the **UPDATE_LOCATION** contained the update 0001, by running this command, you will create a new zip file called **WSO2-CARBON-UPDATE-4.4.0–0001.zip** in the current working directory. Platform Version and Update Number are read from the **update-descriptor.yaml** file. If there are any removed files, you have to open this zip file and add that entry to the **update-descriptor.yaml** file manually.

**NOTE:** You can run `wum-uc --help` get a list of available commands. Also you can run `wum-uc create --help` to find out more about the create command.

#### validation command

After we create a update, we might want to unzip it and add more detail to the **update-descriptor.yaml** like removed files. After we do these changes, we can use this validation command to verify that the file structure of the zip is is the same as the distribution.

```bash
wum-uc validate <update_loc> <dist_loc> [<flags>]

<update_loc> - Location of the update. This should be a zip file.
<dist_loc> - Location of the distribution zip file.
<flags> - Flags for the tool. Currently, supported flags are -d and -t which will print debug logs, trace logs.
```

This will compare the update zip’s directories and files with the distribution’s directories and files.

**NOTE:** Also you can run `wum-uc validate --help` to view the help.

### Creating an update for a C5 based product

First of all you need to use the **init** command to initialize an empty directory. As mentioned above this 
will generate the  **update-descriptor.yaml** which you will need to edit manually in order to provide the relevant 
update details like update no, platform version, platform name, fug fixes and description. Upon completion of 
amending relevant details to the **update-descriptor.yaml**, you need to run **generate** command by providing the 
absolute locations of updated pack (with the fix), previous pack (with the bug) and the  update directory which 
**init** 
was performed.

```bash
wum-uc generate <update_dist_loc> <previous_dist_loc> <update_directory>
<update_dist_loc>  - Location of the updated distribution zip with containg update
<previous_dist_loc> - Location of the previous distribution zip
<update_directory> - Location of the update directory
```
This will compare the diff between given two distributions and generate the update zip, which can be applied by 
WUM-client to produce the updated pack at the client side