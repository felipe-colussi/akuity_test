
Problem: https://gist.github.com/jiachengxu/872db564e4261220d63c79adec09da87

# First disclaimer: 

If I had to implement this (on a real enviorment) I would take few minuts to double check the requirements.
I would Try talking myself out of doing this as writen on the problem statement. (Talk more during interview)

I'm Assuming that My ask of removing/changing this requirement would be accepted: 

"NOTE: NamespaceClass should allow creating any kind of resources (not only NetworkPolicy/ServiceAccount)."


I'll write the code if I were able to change this requirement to the decorator should support "pre-defined ressources" and be 
easly extensible (before actual publishing). 


I just got notified from the task on wednesday (12) and during this weekend I'm going back from my home-town to my actualy house (10 h Drive).  My idea will be to do this as mentioned here and if I have extra time to implement the generic implementation, that I hardly Disagree on. 


The proposed spec (in all cases) will be typed like: 

pod []podObject
NetworkPolicy []NetworkPolicy
ServiceAccount []serviceAccountObject
Secret []secretObject


If I have time I'll do extra research to se how to build the CRD with a map[string][]any{} or some similar implementation for generic implementation and add to the repo. It seems that the API has a dynamic.Interface  that would support that case. 


# Definitions and assumptions: 

Based on the problem statement I'm assuming that a Namespace only can have a single "NamespaceClass" attatched to it. I'll be coding/implementing as if that statement is true. That said the design could easly be tweaked to support multiple implementations. 



# Decision Process: 


## Initial though: 

Wile thinking and reading the problem the first solution that come to my mind was a "dumb" solution. And honestly, by reading the problem (With what I'm imagining about its usage) this one would be the best one on my hand. Again, the lack of a 15 min talk to define project objective is haunting me. 



Create a simple CRD: 

spec: 
  supportedType []supportedTypeObject
  supportedType []supportedTypeObject
  supportedType []supportedTypeObject
  

2 Controll lops: 
  - Watching NamespaceClass (CRD) -> Resides on "Admin" namespace.
  - Watching Namespace (Default).

Namespace: 
-- On delete: 
    -- Fetch for ALL NamespaceClass, read Annotations, remove Namespace from It.
-- OnChange (Creation/Update) -- probably gated by label change on controll-loop:
    -- Check for Metadata.label == namespaceclass.akuity.io/name: {CRD_NAME}
        -- If exists:
            -- Fetch All NamespaceClass
                -- Update removing namespace annotation metadata.annotation (WHEN NEEDED)
            -- Update CRD {CRD_NAME} and add (probably to medata.annotation) the namespace.
        -- If does not exist: 
            -- Fetch All NamespaceClass
            -- Update removing namespace annotation metadata.annotation (WHEN NEEDED)

NamespaceClass (CRD):
-- On Creation:
    -- DO NOTHING. (NO requirement to change, we can implement a lookup to already existing annotations).
-- On update:
    -- Define Ressources that are needed. 
    -- fetch ALL SUPPORTED Ressources from metadata.namespaces (using a label search)
    -- delete ressources that are no longuer valid
    -- apply The ones that should remain / exist. We are adding a custom label to it. classobject:1 (Example)


 UPSIDE: 
 Straight forward. 
 Single CRD and 2 loops. 

DOWNSIDE: 
As we use the same CRDs this dosen't scale supper well as we depend on fetching all namespaces/NamespaceClass. 
No way of "modifying" a Class (I'm not sure if this is good or bad :D) (If NS has class it will have the ressources)

Improvement/Update:  Wile writing this we could update to "3 Loops" (or maybe theres a way of doing this in 2 I need to research).
But using predicates we could update the OnUpdate to have 2 cases: 
on annotation (NAMESPACES) update:
    -- Figure out what namespace was added/removed
    -- only implement the changes (apply/create/update) on it. 
        
on spec: 
    -- Run the full flow as described above.
 




## SeconOne thought: 

Its basically an extension of the 1st one with a "second layer on top of it".

We would be adding an extra CRD (Probably would go with a CRD over a configMap) that would contain a mapping between namespace and What is expected to be there. 

This CRD would be something on the lines of: 

name: {NAMESPACE}
type: NamespaceClassStatus
spec:
  target: {NamespaceClass}
  requires_update: false
  status: 
    resource_type1: 
        - name: name 
          // error:  we can extend if we want to have extra behavior
labes:
    target_namespace_class: {namespaceClass}
    

Then we would have the following control loops: 

Namespace:
    -- OnDeletion: 
        -- Delete NamespaceClassStatus
    -- OnChange (Gated?)
        --If annotated: 
            -- Fetch NamespaceClassStatus (on controller/admin NS)
                -- If does not exist
                    -- create one.   
                -- If target exists:
                    -- Check if {NamespaceClass} changed and update it. 


NamespaceClass: 
  -- On Creation:
    -- DO NOTHING. (NO requirement to change, we can implement a lookup to already existing annotations).
  -- OnChange:
    -- fetch NamespaceClassStatus
        -- set requires_update: true

NamepsaceClassStatus:
    -- OnDelete: Delete all resources with corresponding lable on downstream namespace

    -- OnChange:
        -- Fetches target
        -- Compare target against status
        -- Delete / Apply as needed.

### Variation: 
    The variation would be to store the creation objects instead of the NamespaceClass reference. 
    This would allow the user to change something by hand if needed. 


## One night of sleep (Or trying to later): 

Merge both approaches into the dumb approach but with 2 CRDs

NamespaceClass
NamespaceClassStatus (this name is bad)

3 Controll Loops: 
- Namespace:
- NamespaceClass:
- NamespaceClassStatus (Target + NeedsUpdate) -> No Fancy logic for now we could extend depending on the business need.




Namespace:
    -- OnDeletion: 
        -- Delete NamespaceClassStatus // All resources will go away due to the namespace deletion 
    -- OnChange (Gated?)
        --If annotated: 
            -- Fetch NamespaceClassStatus (on controller/admin NS)
                -- If does not exist
                    -- create one.
                    -- Target + NeedsUpdate = true   
                -- If target exists:
                    -- Check if {NamespaceClass} changed and update it + needsUpdate. 

NamespaceClass:
    -- OnDeletion:
        ??? Do nothing? Or delete downstream. Not sure what is the expected behavior. Can se both.
    -- OnChange:
        -- Fetch NamespaceClassStatus (with that class)
        -- Update NeedsUpdate = true

NamespaceClassStatus:
    -- On deletion: 
        -- Fetch All TaggedResources from namespace and delete them.
    -- On Update:
        -- If NeedsUpdate=True
            -- Fetch All Tagged Resources
            -- Fetch NamespaceClass
            -- Reconcile. 


# Test cases: 
A) Namespace1 (Tagging NSC) -> NSC CLASS DOES NOT EXIST 
B) NSC1 -> Should deploy.
C) CRD2 -> Should Deploy and overwrite.
D) Namespace2 Using CRD -> Should deploy
E) Deploy 2nd NSC.  -> No change.
F) Replace 2nd namespace to use it.
