### Description

Update Alertmanager rules through update the mounted rule files. This tool is only for update one rule group , can't add new rule group.

### Work Flow

![](./image/workflow.png)

- Step 1： Modify the rules files；
- Step 2： Reboot container.

### Usage

- Start server example

  ```
  ./server --rulefiles="/home/rulefiles/*.rule.yaml"
  ```

- Start client example

  ```
  ./client --rulefile=="/home/updatefile.yaml" -t "10.10.10.1:2000"
  ```
  
  Get info:

  ```
  ./client --show all -t "10.10.10.1:2000"
  ./client --show ${rule_group_name} -t "10.10.10.1:2000"
  ```
  
  