---
title: "Data Abstractions at Scale (Video Transcript)"
sidebar_label: "Video: Data Abstractions"
sidebar_position: 98
tags: [netflix, video, transcript, abstractions]
---

:::warning
This is a raw transcript from a conference talk. Content may be unformatted.
:::

This video explains how Netflix uses data abstraction layers to efficiently scale its applications and manage vast amounts of data across various use cases.

The speaker, Vidya Arind, a staff engineer at Netflix, discusses:

The problem: Thousands of applications needing to interact with different storage engines, leading to complexity, varied APIs, and isolation issues (1:31).
The solution: Data Abstraction Layers: Introducing an extra layer of interaction between client applications and storage engines to simplify operations and provide a common interface (2:07).
Three key concepts:
Virtualization: Breaking down complex systems, defining clear boundaries, and being able to switch or compose implementations (2:41). This includes sharding for isolation (4:00), composition to build complex abstractions (4:25), and configuration to deploy these compositions (6:05).
Abstraction: Making the system storage agnostic by providing a unified API (e.g., put, get, scan, delete) to clients, regardless of the underlying database (10:30). It also helps ease data migrations through shadow writing (13:36).
Clean APIs: Ensuring that the client-facing API is simple and consistent, abstracting away the underlying complexities (15:02). The video provides an example of a key-value abstraction as a two-level hashmap (15:46).
For the full transcript, please check the video's description!


0:07
I have a lot to cover I'm going to
0:09
Breeze through this I have like 70
0:11
slides uh
0:14
like really bad okay how to efficiently
0:18
scale when there are thousands of
0:20
applications Netflix looks like this uh
0:23
we stream everywhere throughout the
0:25
world wherever the those red marks are
0:27
uh except two places you can see that in
0:29
Gray uh when that's the scale that
0:31
you're
0:32
streaming that's a lot of data I'm Vidya
0:36
Vidya arind um I'm sta stuffer engineer
0:38
at data platform at Netflix and a
0:40
founding member of data abstractions now
0:42
you know why I want to talk about data
0:44
abstractions can we scale for all our
0:47
data use cases that's the question um
0:50
when especially our use cases looks like
0:53
this uh you have key value uh use cases
0:56
your time series use cases analytics use
0:58
search use cases some times key value
1:01
has larger payloads which becomes file
1:03
system or blob blob store use cases
1:05
right and sometimes there are no
1:07
Solutions Netflix is everywhere um uh
1:10
use cases are everywhere in Netflix like
1:12
this and Netflix also has no solutions
1:15
for some of the use cases
1:17
right can we take these common patterns
1:21
and provide a common
1:23
solution can these Solutions be generic
1:26
and storage
1:28
agnostic
1:31
our applications uh if we don't have uh
1:34
abstractions looks like this every
1:36
application needs to understand how the
1:38
storage engine operates apis are uh
1:41
storage engine apis are different right
1:44
uh every application connects to the
1:45
storage engines and has some common
1:48
information like uh it has different
1:50
languages it has different rough edges
1:53
and tuning parameters and cost model is
1:55
totally different from all of these
1:57
databases uh your application is Con
2:00
into this databases is there any
2:01
isolation that you're building every
2:03
application has to build their own
2:04
isolation layer as well the
2:07
solution for me is data abstraction
2:10
layers right um David Willer um wheeler
2:14
uh rightly told we have we can solve any
2:17
problem by introducing an extra layer of
2:20
interaction um data in in distributed
2:24
systems there are very few good ideas
2:26
right obstruction and virtualization can
2:28
be thought about as to um uh few of
2:31
those great ideas um take a complex
2:33
system uh break it down into smaller
2:35
pieces and clearly Define the boundaries
2:39
that's abstraction it take all of these
2:41
abstractions and uh switch the
2:43
implementations or layer it or compose
2:46
it together that's
2:48
virtualization uh here we are adding
2:50
abstraction layer in front of uh the
2:53
databases and in um uh in front of uh
2:57
after you uh from the client applic
3:00
you're connecting to a obstruction layer
3:02
that's a level of interaction you're
3:04
taking um obstruction uh has three main
3:07
components one is obstruction server
3:08
itself it has a client which sits in
3:11
your client application and it and it
3:13
also has a control plane um operation
3:16
where you're you're abstracting out how
3:18
you're connecting to what database using
3:20
what right um I'm going to talk today
3:23
about three uh important Concepts uh
3:25
virtualization obstruction and clean
3:27
apis how do how do we do uh do these uh
3:30
virtualization also has three main
3:31
Concepts that I want to talk about
3:32
charting composition and
3:35
configuration right um uh when uh all
3:38
these application like thousands of them
3:40
are connecting to a single abstraction
3:43
layer that can be a single point of
3:44
failure you need to talk uh think about
3:47
um how how do you deploy these so that
3:50
we can avoid Noisy Neighbor problems we
3:53
talked about rate limiting before so
3:55
that's also a way of um thinking uh
3:59
thinking about
4:00
isolation um sharding um here if you see
4:04
every uh set of application is
4:06
connecting to its own obstruction Ser
4:09
server right obstruction layer and then
4:12
and the obstruction layer using the
4:14
control plane um knows how to uh talk to
4:17
which database and when to talk to these
4:19
databases that's the isolation we are
4:21
providing by charting um that's charting
4:25
composition um think about abstraction
4:27
as not one one simple server um here I'm
4:31
representing key value obstruction where
4:34
it's a proxy uh is a it's a easy to box
4:37
and inside the in box you have a proxy
4:39
and abstraction code itself um you can
4:43
complicate that by adding a another
4:45
layer this is a tree obstruction for us
4:47
tree obstruction is just nodes and edges
4:50
you can uh think about path enumeration
4:52
technique uh uh as as your obstruction
4:55
and the path ination technique can be uh
4:58
stored in key value abstraction itself
5:01
um you have uh custom apis given by your
5:05
clients that can also be part of your uh
5:08
abstraction layer and um UI
5:10
personalization is an example here you
5:12
can complicate the abstractions even
5:15
more by providing something like this
5:17
where you have a front door connecting
5:20
to uh Journal service which is taking
5:22
all the requests and um kind of adding
5:25
Audits and uh per request um data into a
5:29
Time series
5:30
uh table that can later be used for um
5:34
uh correcting some of the data if there
5:36
is discrepancies in data as well and
5:38
there's storage engine which is
5:39
connecting to key value abstraction
5:41
other databases and search engine as
5:44
well you can also use abstractions for
5:47
uh Shadow writing um when you have
5:50
migrations to do in this case you have a
5:53
um Thrift um container and a cql
5:56
container connecting to different
5:57
databases and um my we'll talk about
6:00
migrations a little bit later that's
6:02
composition all of this composition
6:05
needs some kind of a configuration that
6:08
that needs to be plugged in
6:11
right um You can write those
6:13
configuration down and Define how you
6:16
want to compose these things right use
6:18
configuration to deploy is the next
6:21
topic uh there I'm going to talk about
6:23
two configurations here one is the
6:25
runtime configuration where you're uh
6:28
literally saying how do you compose
6:30
these uh different um abstractions
6:32
together here I have key value
6:34
abstraction um you have uh key value
6:38
obstructions but in the two flavors uh
6:40
it's uh you have an expression and the
6:42
scope which defines uh which
6:44
configuration it'll use to connect to
6:46
which database right uh there is a
6:48
thrift um container and a KV container
6:51
um it's wired through K Thrift as a
6:54
primary and KV as a um uh shadow shadow
6:59
right and reads right um and you can see
7:02
on your right that uh uh this
7:05
configuration is translated into what is
7:08
deployed in your
7:09
right um namespace is another concept
7:13
that I want to talk about namespace is
7:14
an abstraction on top of uh what storage
7:19
inin you're using it's a basic um
7:21
obstruction concept where Nam space is
7:24
uh a string that you define um and you
7:27
have configuration for each of these
7:29
names spaces uh in here I have version
7:32
and scope you can also see um like
7:35
physical storage uh this uh namespace is
7:38
connecting to Cassandra or evach um and
7:42
you have Cassandra and evach
7:44
configuration uh in place you can also
7:46
store uh consistency scope and target
7:49
for the each of these um conf each of
7:51
these data stores like for example read
7:54
your right here uh would translate for
7:56
Cassandra into a local quorum uh reads
8:00
and writes right so uh you can abstract
8:03
out all the information uh from the
8:06
client is what uh the namespace provides
8:09
us um exactly so uh uh this is a watch
8:13
namespace is a control plane API uh
8:16
given a Shard identity it'll return the
8:18
list of name spaces um control plane is
8:21
what is talking to the obstruction
8:23
server and giving us um uh all that we
8:26
need to know which database to connect
8:29
to
8:30
the control plane itself can be an
8:32
abstraction right like it's just uh at
8:35
the end of the day how we deploy how we
8:37
write it down so control plane here is
8:40
uh is deployed as a abstraction control
8:43
plane client lives in the obstruction
8:45
servers instead of the client
8:47
applications and uh that's the
8:49
difference it's talking to a uh config
8:52
store uh it uses long pooling to pull
8:55
for uh new name spaces that appear into
8:57
the control plane that helps us do um
9:00
immediately uh create sessions and
9:03
prepared statements and warm up your
9:06
queries um before the obstruction starts
9:09
or when the new name spaces are
9:12
added uh we we do a bunch of things with
9:15
uh control plane itself like um you use
9:17
temporal workflows or spinco pipelines
9:20
and uh use Python code a little bit to
9:24
deploy or create new name spaces and all
9:27
the artifacts related to the namespaces
9:29
for examp example tables and clusters
9:31
and things like that um watch name space
9:34
this is an important API for control
9:36
plane um watch names space uh I'm good
9:39
on time okay watch Nam space uh has uh
9:43
takes in uh short identity and the last
9:45
seen version and if there's a new
9:47
version it returns back um uh the list
9:50
of name spaces uh with with the version
9:53
that it sees U clone namespace is a
9:56
little bit different concept where you
9:58
are taking a conf configuration of a
9:59
source namespace and creating a target
10:02
namespace with a new artifacts like new
10:04
candra cluster or um a new table in
10:08
inside a Centra cluster it it's a
10:10
asynchronous process where you are
10:13
waiting for things to be done that's why
10:15
you get a job ID back that um concludes
10:19
my virtualization it's just not limited
10:21
to what I talked about but there's more
10:23
you can um derive from this um so the
10:27
sorry uh the next concept I want to talk
10:30
about is abstraction itself um I want to
10:33
talk two main things about abstraction
10:35
there's tons more to talk about uh I I
10:38
think given the time I think two is is a
10:41
good uh good compromise um storage
10:44
agnostic how do you make uh obstruction
10:47
storage agnostic and dual rights um uh
10:51
everybody here has done migration at
10:53
some point I'm I'm hoping um the main
10:57
obstruction I'm going to talk about is
10:58
key value obstruction here um uh David
11:01
rightly pointed out API is your contract
11:04
with a CL uh client that you have to
11:07
solidify your apis what you do in the
11:10
back end is just abstracted out from the
11:13
customers right uh you can see your put
11:16
get scan delete is the contract that you
11:18
are giving to the customers which is the
11:20
client applications what happens uh how
11:23
we call the underlying apis of each and
11:27
every database is agnos here right um uh
11:31
in this case when I talk to mcash D um I
11:34
use sets and gets in Cassandra I use
11:37
selects and inserts and Dynamo DB I use
11:40
um put put and
11:42
queries uh in obstruction layer itself
11:45
we have different data stores or record
11:47
stores which uh which helps us um
11:50
abstract out that details and control
11:52
plane has the information about which
11:54
database to connect to when the request
11:56
comes in for a specific name space um uh
11:59
and depending on the record store that
12:01
the control plane dictates uh you
12:03
connect to that particular database um
12:06
with the record store
12:07
information uh again it's a namespace
12:10
one here in your left has a see uh it's
12:13
connecting to a Cassandra database and
12:15
namespace 2 here um connects to the
12:17
Dynamo DB database is just how it's
12:20
configured right uh the request comes in
12:22
with the namespace name that's how you
12:24
know where to connect
12:25
to that makes it storage agnostic right
12:29
um as I said everybody almost I think
12:33
would have gone through migrations and
12:34
migrations are painful right um how can
12:38
abstraction help ease this Spain uh last
12:40
year we ran a program for uh convert uh
12:44
moving from Thrift to cql at Netflix it
12:47
took almost a a year and a half for us
12:50
to migrate like 250 Cassandra clusters
12:53
from three uh 2 Cassandra to 30
12:56
Cassandra right um that
13:00
plus some use cases like these right um
13:04
where uh user comes to us um and says I
13:09
I only have a gigabyte of data and
13:12
quickly in a year or so realizes oh I I
13:15
I have to store more data Json BL uh I
13:18
want to store a larger Json blob I I I
13:21
want to uh store just not use the
13:24
database I have right uh all of these
13:27
requires migration uh use case starts as
13:30
a simple key value quickly moves into a
13:33
blob
13:34
store we need to
13:36
migrate migration for us looks like this
13:39
right um uh the client API is always the
13:42
key value API um or whatever the
13:45
obstruction they're using um we migrate
13:48
um uh we start like this DB1 is your one
13:51
implementation um we add db2 which is
13:55
our sha which is in the shadow right
13:57
mode I talked about Shadow right earlier
13:59
we now are talking to both the databases
14:02
parall um uh underneath we move the data
14:06
from DB1 to db2 backfilling the data
14:09
from DB1 to D db2 all of this happens
14:12
without client even touching anything in
14:14
their site and we promote db2 as a
14:17
primary and DB1 is still getting the
14:19
data but it's uh is just in the shadow
14:23
mode um and then we go on decommission
14:26
DB1 at that point all the trace of old
14:30
uh databases gone right um uh persistent
14:34
config for dual rights looks something
14:36
like this um you have uh the same
14:39
namespace names space one um having two
14:42
persistent config we talked about SC
14:44
scope a little bit earlier I probably
14:46
Breeze through it faster scope is how we
14:49
are um uh telling the
14:51
container uh scope this configuration to
14:54
that particular
14:56
container great uh that's up obstruction
14:59
for us um all this is coming to is a
15:02
clean API uh no matter how many
15:05
obstructions you have you have to have a
15:07
clean and simple API in the client's
15:09
side so that they come and use your
15:11
abstractions one and uh it's simpler for
15:15
them to move and they're not aware of
15:17
all the um Oddities that are happening
15:19
in the end right um for key value
15:21
obstruction that I'm going to talk about
15:23
today uh simple API like you can you can
15:27
almost think of this as like a Java apis
15:29
right there's no almost no difference
15:31
between puts gets um gets mutate mutate
15:35
is little different but M
15:37
mutates um scan put a faps and or
15:40
compute those are the simple apis um to
15:44
uh think about uh key value obstruction
15:46
think about it as uh two level hashmap
15:50
right um one level is your IDs and the
15:52
second level is a sorted map of bytes
15:55
and
15:56
bytes uh so uh key Val storage layer
16:00
looks uh the base table looks like this
16:02
where when when you have a simple
16:03
payload it's ID and key key is a
16:06
clustering column which is the sorted
16:08
sorted map I talked about and the value
16:10
um I have I I'll come back to the value
16:13
metadata in a bit item itself looks very
16:16
simple bytes right a key key is a bite
16:19
values a bite and a some uh value met
16:22
data if
16:23
the value itself is a large uh value
16:26
then you uh you need chunking uh chunk
16:28
chunk information as well metadata is a
16:32
little complicated uh concept uh but uh
16:35
there are other talks you can go listen
16:36
to about metadata um one is U
16:40
compression you want to do client side
16:42
compression and when you do client side
16:44
compression you need to store which
16:46
algorithm did you use to compress the
16:47
data kind of information so that's uh uh
16:51
compression life cycle right times
16:53
expired times uh Etc can be stored in
16:56
life cycle metadata chunk metadata where
16:59
is the chunk how many chunks did you
17:01
make of the payload um where is it
17:03
stored and what hash algorithms did you
17:05
use to chunk all of that is stored in
17:08
chunk metadata content metadata is how
17:10
you're rendering that uh data back to
17:13
the client that's uh metadata put calls
17:15
very simple you have a namespace here uh
17:19
you can see how the requests are coming
17:21
through to the service through uh simple
17:24
namespace as the abstraction uh part
17:27
right like using a namespace you can
17:29
again find out who who call which uh
17:33
service to call or which database to
17:35
call um ID and a list of items um item
17:38
poy token is very interesting um I uh it
17:41
is generated in the client side for us
17:44
uh client side generates a monotonically
17:45
increasing item proty token and attaches
17:48
it to every put call and get calls um
17:52
mutation is list of puts and gets uh we
17:55
order these puts and gets in uh in the
17:57
same mutation request using the item
17:59
Pary token again we have item token.
18:01
next which will give you a monotonically
18:04
increasing number um it's interesting uh
18:07
get items is given a name space and an
18:10
ID with the predicate which which uh
18:13
dictates match all um match a list of
18:16
keys or uh match a range of queries um
18:21
it'll return a list of items and the
18:22
next page token which will help you Pate
18:25
through the whole list scan uh this
18:28
again is uh very interesting for
18:30
migrations um scan uh given a name space
18:33
we want to scan the whole uh whole table
18:36
or whole name space with all the data
18:38
like it can have have hundreds or in a
18:41
billion and you want to paginate those
18:43
data so you what is returned is a scan
18:45
result with the next page token where
18:48
you p through the tokens um I want to uh
18:53
go through this but let's let's see if I
18:54
can make it I have a small amount of
18:58
time if the payload is small less than 1
19:00
MB there is nothing you can uh you need
19:03
to do just store it straight to the
19:05
database right what else um if your
19:08
payload is large like 4 MB then you have
19:11
to chunk the data when you chunk the
19:13
data you chunk it into 64 KB um of
19:17
chunks and you stream the chunks into
19:19
the server or commit it to the server
19:22
after you commit you commit chunk zero
19:25
chunk zero is what is determining have
19:28
you done the all the work that is needed
19:30
to so that the um the data is visible to
19:33
the customer right um in the read path
19:36
you first read chunk zero determine
19:38
where your chunks are and then go fetch
19:41
parallely all the
19:42
chunks and um Stitch it together the
19:45
chunks for us lives in a data table um
19:48
when when you uh determine It's a larger
19:51
payload you uh store the value metadata
19:53
in the base table and value is empty and
19:56
value metadata determines where the data
19:58
is um we uh spread the load of uh
20:02
different chunks using bucketing um
20:04
bucketing strategy ID is your primary
20:07
still uh you bucket the data and your
20:09
key is uh sorted uh key chunk and
20:12
version ID are uh the clustering columns
20:16
right um okay uh I am I am going to be
20:19
done in one minute okay
20:23
uh so this is how you spread the load um
20:26
I'm going to I think that con includes
20:29
how my API uh looks there are a lot of
20:32
building blocks you can see for key
20:34
value abstraction we have done chunking
20:37
compression adaptive pagination caching
20:40
signaling SLO signaling summarization
20:42
there are tons of features you add and
20:46
abstraction with many tunable features
20:48
is what I want to leave you with uh more
20:52
abstractions yeah this is just a general
20:54
concept right like you can build many
20:56
abstractions with that you can see
20:59
uh there are tons that we have built and
21:02
we are adding more as well so for
21:04
example uh time uh uh uh key values are
21:08
oldest obstruction with 400 charts in it
21:11
uh time series counter identifier entity
21:14
tree graph Q you can keep going I'll
21:18
leave you with Mark Anderson's code
21:20
every new layer of obstruction is a new
21:22
chance for a clean slate redesign of
21:24
everything making everything little
21:26
faster less power hungry more elegant
21:29
easy to use and
21:31
cheaper thank
21:33
[Applause]
21:42
you


