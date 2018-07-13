# Update Creator Tool

### Introduction

Update Creator is a tool which helps to create new updates which are compatible with **WUM**. This tool is written in GO
 language. One of the main advantages of writing this tool in GO is that we can compile the code directly to machine code so we don’t need something like JVM to run this code. Another advantage is we can cross compile the code.

This tool mainly provides 2 functions.

1. Creating a new update
2. Validating an Update

### Installation

First you need to install GO to compile and run this tool. You can find instructions to how to download and install the GO from the [Official Website](https://golang.org/doc/install). 

Then run the following command.

`go get -u github.com/wso2/update-creator-tool`

This will download and install the packages along with their dependencies. 

Then run `build.sh`. This will generate the executable files for various OS/Architecture combinations. These will be located at **/build/target/** directory. Extract the relevant zip file to your OS/Architecture. In the *bin* directory, you'll find the executable **wum-uc** file. 

### Adding the tool to system path variables

By adding the **wum-uc** tool path to system path variables, you’ll be able to run the tool from anywhere. Command to
add the tool path to system path variable in Ubuntu is shown below.

`export PATH=$PATH:[ENTER_PATH_TO_BIN_HERE]`


### Initializing wum-uc tool

First, you need to initialize **wum-uc**  by running the following command,

```
wum-uc init
```

Which will prompt you for WSO2 username and password.

Upon initializing, **wum-uc**  will create a **.wum-uc** directory ( referred later as $WUMUC_HOME) in your home
directory.


### Creating an update

This is done using the `create` command. Please follow below steps for creating an update.

1. First, you need to create an empty directory to gather all the files related to the update. Let’s call this `$UPDATE_LOCATION`.
2. Upon creation of `$UPDATE_LOCATION` directory copy all the files that needs to be in your update.
   So `$UPDATE_LOCATION` should contain:
   * All of the updated files (ie. binary, resource files).
   * instructions.txt (optional, required only till **WUM 2.0**  gets officially depreciated )

   **NOTE:** Please do not copy `LICENSE.txt` or `NOT_A_CONTRIBUTION.txt` to the `$UPDATE_LOCATION` as they get automatically downloaded and added to the update by wum-uc tool.
3. Once all the required files are added to `$UPDATE_LOCATION` run the following command to create the update.

   ```
   wum-uc create <update_dir> <dist_loc>

   <update_dir> : path to the $UPDATE_LOCATION
   <dist_loc>	: path to the latest wum updated distribution which you are creating the update
   ```

   **NOTE:** In above please use the latest wum updated distribution obtained by pointing wum to the live environment. ( ie. `url: https://api.updates.wso2.com`)
4. Provide the relevant update number when prompted as follows.
   ```
   Enter 'update number':
   ```
5. Select the relevant platform version which you are creating the update for when prompted as follows,

   ```
   Select the platform name and version from following:
   	1. wilkes 	 4.4.0
   	2. hamming 	 5.0.0
   Enter your preference [1/2]:
   ```
6. When the tool prompt for removed files as follows,

    ```
    Are the existing files in wso2am-2.1.0.1529382635299 removed from this update? [y/n]
    ```

    Press `n` if no files are removed from this update, or else press `y` for adding removed files. If `y`, please enter the path of the removed files relative to the PRODUCT_HOME.

    ```
    Enter the path of a removed file relative to the PRODUCT_HOME, press enter when the path is added
    ```

	The tool will prompt you for multiple removed files and when you are done with adding removed files, press `Enter` key without any inputs. The tool will detect the empty input and ask you for confirmation as follows,

    ```
    Empty input detected, are you done with adding inputs? [y/n]:
    ```

	Press `y` if you are done with adding removed files or else press `n` for adding more removed files.

    (**NOTE:** the following 7, 8, 9 will only be prompted till **WUM 2.0**  get officially depreciated.)
7. Provide names of product/s for the `applies to` field when prompted as follows,

   ```
   Enter applies to:
   ```
8. Provide JIRA keys and summaries relevant to the update when prompted as follows,
   ```
    Enter Bug fixes,
   	Enter JIRA_KEY/GITHUB ISSUE URL: DEMOTESTPROD-13
   	Enter JIRA_KEY_SUMMARY/GITHUB_ISSUE_SUMMARY for 'DEMOTESTPROD-13' : sample summary
   ```

   Same as in step 6, the tool will prompt you for multiple inputs, please provide an empty input (press `Enter` key) when you are done with adding relevant JIRA KEYS.
9. Provide the description for the created update when prompted as follows,

   ```
   Enter the description:
   ```

   (**NOTE:** the above  7, 8, 9 will only be prompted till **WUM 2.0**  get officially depreciated.)
10. Once done, update zip will be created at the location where you execute **wum-uc** and the tool will display summary
 of the update creation process. A sample will be as follows,

    ```
    'update-descriptor.yaml' has been successfully created in '/home/kasun/Documents/wum-uc/demo'.
    Optional resource file 'instructions.txt' not copied.
    'update-descriptor3.yaml' has been successfully created in '/home/kasun/Documents/wum-uc/demo'.
    'WSO2-CARBON-UPDATE-4.4.0-2923.zip' successfully created.

    Your update applies to the following products
    	Compatible products : [wso2am]
    	Partially applicable products : [wso2esb wso2is-km wso2am]
    	Notify products : [wso2ei wso2iot]
    Manually fill the `description`,`instructions` and `bug_fixes` fields for above products in the update-descriptor3.yaml located inside the created 'WSO2-CARBON-UPDATE-4.4.0-2923.zip'
    ```

    As depicted from the above summary, the created update **2923** gets fully applied to **wso2am** and gets partially
    applied to **wso2esb**, **wso2is-km**, **wso2am** products.

    **NOTE:**  As depicted in `Notify products` field of the above summary the same update can be applied to the products
    **wso2ei** and **wso2iot**  as well, however due to the difference in the directory structure of above products it
    is the responsibility of the developer to create seperate updates for the above listed products (in `Notify products` field).

Some samples for the **UPDATE_LOCATION** directory is shown below.

**Sample 1**
```bash
├── axis2_1.6.1.wso2v16.jar
├── instructions.txt
├── LICENSE.txt
├── NOT_A_CONTRIBUTION.txt
├── synapse-core_2.1.5.wso2v2.jar
├── update-descriptor3.yaml
└── update-descriptor.yaml
```

**Sample 2**
```bash
├── LICENSE.txt
├── NOT_A_CONTRIBUTION.txt
├── oauth2.war
├── update-descriptor3.yaml
└── update-descriptor.yaml
```

**Sample 3**
```bash
├── LICENSE.txt
├── NOT_A_CONTRIBUTION.txt
├── org.wso2.carbon.apimgt.hostobjects_5.0.3.jar
├── store
│   ├── modules
│   │   └── subscription
│   │       ├── list.jag
│   │       └── module.jag
│   └── site
│       └── blocks
│           └── subscription
│               └── subscription-list
│                   ├── ajax
│                   │   └── subscription-list.jag
│                   └── block.jag
├── update-descriptor3.yaml
└── update-descriptor.yaml
```

**Sample 4**
```bash
├── bin
│   └── tomcat-juli-7.0.69.jar
├── lib
│   └── endorsed
│       └── tomcat-annotations-api-7.0.69.jar
├── LICENSE.txt
├── org.wso2.carbon.tomcat_4.4.3.jar
├── tomcat_7.0.59.wso2v3.jar
├── tomcat-catalina-ha_7.0.59.wso2v1.jar
├── tomcat-el-api_7.0.59.wso2v1.jar
├── tomcat-jsp-api_7.0.59.wso2v1.jar
├── tomcat-servlet-api_7.0.59.wso2v1.jar
├── NOT_A_CONTRIBUTION.txt
├── update-descriptor3.yaml
└── update-descriptor.yaml
```

### Command Reference

You can run **wum-uc** in the terminal to view available commands and help. Since you have added the bin directory to
system path, you can call this command from anywhere.
You can run `wum-uc <command> --help` to view the help of each command.

#### init command

This command will initialize `wum-uc` with your WSO2 credentials.

#### create command

This command will create a new update.

```
wum-uc create <update_loc> <dist_loc> [<flags>]

<update_loc> - Location of the updated files.
<dist_loc> - Location of the product distribution zip file.
<flags> - Flags for the tool. Currently, supported flags are -d and -t which will print debug logs, trace logs.
```
This command will prompt for required user inputs and generate the **update-descriptor.yaml** (until **WUM 2.0** gets
officially depreciated), **update-descriptor3.yaml** files and download **LICENSE.txt** and **NOT_A_CONTRIBUTION.txt**
for creating the update. If there is a **README.txt** file in the old patch format in the **<update_loc>** directory,
this command will try to parse the necessary details from the **README.txt** file and use them to populate
**update-descriptor.yaml** and **update-descriptor3.yaml** files (only `update_number`, `platform_version` and
`platform_name` fields will get populated in **update-descriptor3.yaml**). Otherwise, the tool will prompt for inputs
from the user.

**NOTE:** You can run `wum-uc --help` get a list of available commands. Also, you can run `wum-uc create --help` to
find
 out more about the create command.

**NOTE:** You can run `wum-uc --help` get a list of available commands. Also you can run `wum-uc create --help` to find out more about the create command.

#### validation command

After we create an update, it is required to unzip it and fill in the `description`, `instructions` and `bug_fixes`
fields of the created **update-descriptor3.yaml** relevant to the products listed in the update creation summary shown
as the final output of `wum-uc create` command (like the sample provided in step no 10 above).

After performing these changes, we can use the validation command to verify that the file structure of the zip is the same as the distribution.


```
wum-uc validate <update_loc> <dist_loc> [<flags>]

<update_loc> - Location of the update. This should be a zip file.
<dist_loc> - Location of the distribution zip file.
<flags> - Flags for the tool. Currently, supported flags are -d and -t which will print debug logs, trace logs.
```

This will compare the update zip’s directories and files with the distribution’s directories and files.

**NOTE:** Also you can run `wum-uc validate --help` to view the help.
