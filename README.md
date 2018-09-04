# eksClient
This repository shows how it is possible to use a lambda function
to control an EKS cluster.  This code is supporting this [blog](http://www.nickaws.net/aws/2018/08/26/Interacting-with-EKS-via-Lambda.html) entry.

* Version 0.1 - Functionality Demonstrated
* Version 0.2 - Performance and stabilty impovements:
.. - Moved back to aws-sdk-go version 1 for stability
..- Embedded aws-iam-authenticator pkg into code instead of downloading external binary into runtime environment
.. - Run time decreased from 5s to approx 1-2s
.. - saved original effort as branch version1
.. - using client-go version 8 (1.11 compatible)
.. - added region selector using ENV variable
